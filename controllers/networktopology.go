package controllers

import (
	"context"
	"errors"
	"time"

	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"
	"github.com/giantswarm/microerror"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	capiconditions "sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
	nettopAnnotations "github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
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
	client     ClusterClient
	registrars []Registrar
}

func NewNetworkTopologyReconciler(client ClusterClient, registrars []Registrar) *NetworkTopologyReconciler {
	return &NetworkTopologyReconciler{
		client:     client,
		registrars: registrars,
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

	if annotations.IsPaused(cluster, cluster) {
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

	for _, reg := range r.registrars {
		err = reg.Register(ctx, cluster)
		if err != nil {
			if errors.Is(err, &registrar.ModeNotSupportedError{}) {
				capiconditions.MarkFalse(cluster, networkTopologyCondition, "ModeNotSupported", capi.ConditionSeverityInfo, "The provided mode '%s' is not supported", nettopAnnotations.GetAnnotation(cluster, gsannotation.NetworkTopologyModeAnnotation))
				return ctrl.Result{Requeue: false}, nil
			} else if errors.Is(err, &registrar.TransitGatewayNotAvailableError{}) {
				capiconditions.MarkFalse(cluster, networkTopologyCondition, "TransitGatewayNotAvailable", capi.ConditionSeverityWarning, "The transit gateway is not yet available for attachment")
				return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 1}, nil
			} else if errors.Is(err, &registrar.VPCNotReadyError{}) {
				capiconditions.MarkFalse(cluster, networkTopologyCondition, "VPCNotReady", capi.ConditionSeverityInfo, "The cluster's VPC is not yet ready")
				return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 1}, nil
			} else if errors.Is(err, &registrar.IDNotProvidedError{}) {
				capiconditions.MarkFalse(cluster, networkTopologyCondition, "RequiredIDMissing", capi.ConditionSeverityError, "The %s ID is missing from the annotations", err.(*registrar.IDNotProvidedError).ID)
				return ctrl.Result{Requeue: false}, nil
			}

			return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 10}, microerror.Mask(err)
		}
	}

	capiconditions.MarkTrue(cluster, networkTopologyCondition)
	return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 10}, nil
}

func (r *NetworkTopologyReconciler) reconcileDelete(ctx context.Context, cluster *capi.Cluster) (ctrl.Result, error) {
	if !r.client.ContainsFinalizer(cluster, FinalizerNetTop) {
		return ctrl.Result{}, nil
	}

	for i := range r.registrars {
		registrar := r.registrars[len(r.registrars)-1-i]

		err := registrar.Unregister(ctx, cluster)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	err := r.client.RemoveFinalizer(ctx, cluster, FinalizerNetTop)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{}, nil
}
