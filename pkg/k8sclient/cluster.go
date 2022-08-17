package k8sclient

import (
	"context"

	"github.com/giantswarm/microerror"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Cluster struct {
	client client.Client
}

func NewCluster(client client.Client) *Cluster {
	return &Cluster{
		client: client,
	}
}

func (g *Cluster) Get(ctx context.Context, namespacedName types.NamespacedName) (*capi.Cluster, error) {
	Cluster := &capi.Cluster{}
	err := g.client.Get(ctx, namespacedName, Cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return Cluster, microerror.Mask(err)
}

func (g *Cluster) AddFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	originalCluster := capiCluster.DeepCopy()
	controllerutil.AddFinalizer(capiCluster, finalizer)
	return g.client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster))
}

func (g *Cluster) RemoveFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	originalCluster := capiCluster.DeepCopy()
	controllerutil.RemoveFinalizer(capiCluster, finalizer)
	return g.client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster))
}
