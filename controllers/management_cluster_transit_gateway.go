package controllers

import (
	"context"

	"github.com/giantswarm/microerror"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	capiannotations "sigs.k8s.io/cluster-api/util/annotations"
	capiconditions "sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/conditions"
)

type ManagementClusterTransitGatewayReconciler struct {
	client ClusterClient
}

func NewManagementClusterTransitGateway(client ClusterClient) *ManagementClusterTransitGatewayReconciler {
	return &ManagementClusterTransitGatewayReconciler{
		client: client,
	}
}

func (r *ManagementClusterTransitGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}

func (r *ManagementClusterTransitGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling")
	defer logger.Info("Done reconciling")

	cluster, err := r.client.GetAWSCluster(ctx, req.NamespacedName)
	if k8sErrors.IsNotFound(err) {
		logger.Info("Cluster no longer exists")
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	defer func() {
		_ = r.client.UpdateStatus(ctx, cluster)
	}()

	if capiannotations.HasPaused(cluster) {
		logger.Info("Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	if !capiconditions.Has(cluster, networkTopologyCondition) {
		capiconditions.MarkFalse(cluster, networkTopologyCondition, "InProgress", capi.ConditionSeverityInfo, "")
	}

	if !cluster.DeletionTimestamp.IsZero() {
		logger.Info("Reconciling delete")
		return r.reconcileDelete(ctx, cluster)
	}

	return r.reconcileNormal(ctx, cluster)
}

func (r *ManagementClusterTransitGatewayReconciler) reconcileNormal(ctx context.Context, cluster *capa.AWSCluster) (ctrl.Result, error) {
	err := r.client.AddFinalizer(ctx, cluster, FinalizerNetTop)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	conditions.MarkReady(cluster, conditions.TransitGatewayCreated)
	return ctrl.Result{}, nil
}

func (r *ManagementClusterTransitGatewayReconciler) reconcileDelete(ctx context.Context, cluster *capa.AWSCluster) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
