package controllers

import (
	"context"
	"time"

	"github.com/giantswarm/microerror"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const FinalizerNetTop = "network-topology.finalizers.giantswarm.io"

//counterfeiter:generate . ClusterClient
type ClusterClient interface {
	Get(context.Context, types.NamespacedName) (*capi.Cluster, error)
	AddFinalizer(context.Context, *capi.Cluster, string) error
	RemoveFinalizer(context.Context, *capi.Cluster, string) error
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
		if errors.IsNotFound(err) {
			logger.Info("Cluster no longer exists")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, microerror.Mask(err)
	}

	if annotations.IsPaused(cluster, cluster) {
		logger.Info("Infrastructure or core cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	return r.reconcileNormal(ctx, cluster)
}

func (r *NetworkTopologyReconciler) reconcileNormal(ctx context.Context, Cluster *capi.Cluster) (ctrl.Result, error) {
	err := r.client.AddFinalizer(ctx, Cluster, FinalizerNetTop)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	for _, registrar := range r.registrars {
		err = registrar.Register(ctx, Cluster)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	return ctrl.Result{Requeue: true, RequeueAfter: time.Minute * 10}, nil
}

func (r *NetworkTopologyReconciler) reconcileDelete(ctx context.Context, Cluster *capi.Cluster) (ctrl.Result, error) {
	for i := range r.registrars {
		registrar := r.registrars[len(r.registrars)-1-i]

		err := registrar.Unregister(ctx, Cluster)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	err := r.client.RemoveFinalizer(ctx, Cluster, FinalizerNetTop)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{}, nil
}
