package controllers

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

const FinalizerResourceShare = "network-topology.finalizers.giantswarm.io/share"

//counterfeiter:generate . RAMClient
type RAMClient interface {
	ApplyResourceShare(context.Context, aws.ResourceShare) error
	DeleteResourceShare(context.Context, string) error
}

type ShareReconciler struct {
	ramClient     RAMClient
	clusterClient ClusterClient
}

func NewShareReconciler(clusterClient ClusterClient, ramClient RAMClient) *ShareReconciler {
	return &ShareReconciler{
		ramClient:     ramClient,
		clusterClient: clusterClient,
	}
}

func (r *ShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}

func (r *ShareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.getLogger(ctx)

	logger.Info("Reconciling")
	defer logger.Info("Done reconciling")

	cluster, err := r.clusterClient.Get(ctx, req.NamespacedName)
	if k8serrors.IsNotFound(err) {
		logger.Info("cluster no longer exists")
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "failed to get cluster")
		return ctrl.Result{}, err
	}

	if !cluster.DeletionTimestamp.IsZero() {
		logger.Info("Reconciling delete")
		return r.reconcileDelete(ctx, cluster)
	}

	return r.reconcileNormal(ctx, cluster)
}

func (r *ShareReconciler) reconcileDelete(ctx context.Context, cluster *capi.Cluster) (ctrl.Result, error) {
	logger := r.getLogger(ctx)

	err := r.ramClient.DeleteResourceShare(ctx, getResourceShareName(cluster))
	if err != nil {
		logger.Error(err, "failed to apply resource share")
		return ctrl.Result{}, err
	}

	err = r.clusterClient.RemoveFinalizer(ctx, cluster, FinalizerResourceShare)
	if err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ShareReconciler) reconcileNormal(ctx context.Context, cluster *capi.Cluster) (ctrl.Result, error) {
	logger := r.getLogger(ctx)

	transitGatewayAnnotation := annotations.GetNetworkTopologyTransitGatewayID(cluster)

	if transitGatewayAnnotation == "" {
		logger.Info("transit gateway arn annotation not set yet")
		return ctrl.Result{}, nil
	}

	prefixListAnnotation := annotations.GetNetworkTopologyPrefixListID(cluster)

	if prefixListAnnotation == "" {
		logger.Info("prefix list arn annotation not set yet")
		return ctrl.Result{}, nil
	}

	transitGatewayARN, err := arn.Parse(transitGatewayAnnotation)
	if err != nil {
		logger.Error(err, "failed to parse transit gateway arn")
		return ctrl.Result{}, err
	}

	prefixListARN, err := arn.Parse(prefixListAnnotation)
	if err != nil {
		logger.Error(err, "failed to parse transit gateway arn")
		return ctrl.Result{}, err
	}

	accountID, err := r.getAccountId(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	if accountID == transitGatewayARN.AccountID {
		logger.Info("transit gateway in same account as cluster. Skipping")
		return ctrl.Result{}, nil
	}

	err = r.clusterClient.AddFinalizer(ctx, cluster, FinalizerResourceShare)
	if err != nil {
		logger.Error(err, "failed to add finalizer")
		return ctrl.Result{}, err
	}

	err = r.ramClient.ApplyResourceShare(ctx, aws.ResourceShare{
		Name: getResourceShareName(cluster),
		ResourceArns: []string{
			transitGatewayARN.String(),
			prefixListARN.String(),
		},
		ExternalAccountID: accountID,
	})
	if err != nil {
		logger.Error(err, "failed to apply resource share")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ShareReconciler) getAccountId(ctx context.Context, cluster *capi.Cluster) (string, error) {
	logger := r.getLogger(ctx)
	awsCluster := types.NamespacedName{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Spec.InfrastructureRef.Namespace,
	}

	identity, err := r.clusterClient.GetAWSClusterRoleIdentity(ctx, awsCluster)
	if err != nil {
		logger.Error(err, "failed to get AWSCluster role identity")
		return "", err
	}

	roleArn, err := arn.Parse(identity.Spec.RoleArn)
	if err != nil {
		logger.Error(err, "failed to parse aws cluster role identity arn")
		return "", err
	}

	return roleArn.AccountID, nil
}

func (r *ShareReconciler) getLogger(ctx context.Context) logr.Logger {
	logger := log.FromContext(ctx)
	logger = logger.WithName("share-reconciler")
	return logger
}

func getResourceShareName(cluster *capi.Cluster) string {
	return fmt.Sprintf("%s-transit-gateway", cluster.Name)
}
