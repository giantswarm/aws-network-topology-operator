package controllers

import (
	"context"
	"time"

	"github.com/giantswarm/microerror"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	capiannotations "sigs.k8s.io/cluster-api/util/annotations"
	capiconditions "sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/conditions"
	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
)

const (
	FinalizerNetTop                             = "network-topology.finalizers.giantswarm.io"
	networkTopologyCondition capi.ConditionType = "NetworkTopologyReady"
)

//counterfeiter:generate . ClusterClient
type ClusterClient interface {
	Get(context.Context, types.NamespacedName) (*capi.Cluster, error)
	AddFinalizer(context.Context, *capi.Cluster, string) error
	RemoveFinalizer(context.Context, *capi.Cluster, string) error
	ContainsFinalizer(*capi.Cluster, string) bool
	UpdateStatus(ctx context.Context, cluster *capi.Cluster) error
	GetAWSClusterRoleIdentity(context.Context, types.NamespacedName) (*capa.AWSClusterRoleIdentity, error)
}

//counterfeiter:generate . Registrar
type Registrar interface {
	Register(context.Context, *capi.Cluster) error
	Unregister(context.Context, *capi.Cluster) error
}

type NetworkTopologyReconciler struct {
	client    ClusterClient
	registrar Registrar
}

func NewNetworkTopologyReconciler(client ClusterClient, registrar Registrar) *NetworkTopologyReconciler {
	return &NetworkTopologyReconciler{
		client:    client,
		registrar: registrar,
	}
}

func (r *NetworkTopologyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}

func (r *NetworkTopologyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling")
	defer logger.Info("Done reconciling")

	cluster, err := r.client.Get(ctx, req.NamespacedName)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			logger.Info("Cluster no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, microerror.Mask(err)
	}

	if capiannotations.IsPaused(cluster, cluster) {
		logger.Info("Core cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	if !capiconditions.Has(cluster, networkTopologyCondition) {
		capiconditions.MarkFalse(cluster, networkTopologyCondition, "InProgress", capi.ConditionSeverityInfo, "")
		// We're ok to continue if this fails
		_ = r.client.UpdateStatus(ctx, cluster)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		logger.Info("Reconciling delete")
		return r.reconcileDelete(ctx, cluster)
	}

	return r.reconcileNormal(ctx, cluster)
}

func (r *NetworkTopologyReconciler) reconcileNormal(ctx context.Context, cluster *capi.Cluster) (ctrl.Result, error) {
	err := r.client.AddFinalizer(ctx, cluster, FinalizerNetTop)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	defer func() {
		_ = r.client.UpdateStatus(ctx, cluster)
	}()

	err = r.registrar.Register(ctx, cluster)
	if err != nil {
		return markErrorConditions(cluster, err)
	}

	conditions.MarkReady(cluster)
	return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 10}, nil
}

func (r *NetworkTopologyReconciler) reconcileDelete(ctx context.Context, cluster *capi.Cluster) (ctrl.Result, error) {
	if !r.client.ContainsFinalizer(cluster, FinalizerNetTop) {
		return ctrl.Result{}, nil
	}

	err := r.registrar.Unregister(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	err = r.client.RemoveFinalizer(ctx, cluster, FinalizerNetTop)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{}, nil
}

func markErrorConditions(cluster *capi.Cluster, err error) (ctrl.Result, error) {
	// TODO: Log errors

	if registrar.IsModeNotSupportedError(err) {
		conditions.MarkModeNotSupported(cluster)
		return ctrl.Result{Requeue: false}, nil
	}

	if registrar.IsTransitGatewayNotAvailableError(err) {
		// TODO: why no condition here?
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 1}, nil
	}

	if registrar.IsVPCNotReadyError(err) {
		conditions.MarkVPCNotReady(cluster)
		return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 1}, nil
	}

	if registrar.IsIDNotProvidedError(err) {
		id := err.(*registrar.IDNotProvidedError).ID

		conditions.MarkIDNotProvided(cluster, id)
		return ctrl.Result{Requeue: false}, nil
	}

	// TODO: is this necessary? Why do we requeue after 10 minutes for a
	// generic error. I'd rather have the exponential backoff handle this.
	return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 10}, microerror.Mask(err)
}
