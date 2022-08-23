package k8sclient

import (
	"context"

	"github.com/giantswarm/microerror"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Cluster struct {
	client            client.Client
	managementCluster types.NamespacedName
}

// NewCluster returns a new Cluster client
func NewCluster(client client.Client, managementCluster types.NamespacedName) *Cluster {
	return &Cluster{
		client:            client,
		managementCluster: managementCluster,
	}
}

// Get retrieves a Cluster based on the given namespace/name
func (g *Cluster) Get(ctx context.Context, namespacedName types.NamespacedName) (*capi.Cluster, error) {
	cluster := &capi.Cluster{}
	err := g.client.Get(ctx, namespacedName, cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// GetManagementCluster retrieves the Cluster for the management cluster namespace/name provided at client creation
func (g *Cluster) GetManagementCluster(ctx context.Context) (*capi.Cluster, error) {
	cluster := &capi.Cluster{}
	err := g.client.Get(ctx, g.managementCluster, cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// GetAWSCluster retrieves an AWSCluster based on the provided namespace/name
func (g *Cluster) GetAWSCluster(ctx context.Context, namespacedName types.NamespacedName) (*capa.AWSCluster, error) {
	cluster := &capa.AWSCluster{}
	err := g.client.Get(ctx, namespacedName, cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// Save persists changes to the given Cluster
func (g *Cluster) Save(ctx context.Context, cluster *capi.Cluster) (*capi.Cluster, error) {
	err := g.client.Update(ctx, cluster, &client.UpdateOptions{})
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// AddFinalizer adds the given finalizer to the cluster
func (g *Cluster) AddFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	originalCluster := capiCluster.DeepCopy()
	controllerutil.AddFinalizer(capiCluster, finalizer)
	return g.client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster))
}

// RemoveFinalizer removes the given finalizer from the cluster
func (g *Cluster) RemoveFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	originalCluster := capiCluster.DeepCopy()
	controllerutil.RemoveFinalizer(capiCluster, finalizer)
	return g.client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster))
}

// IsManagementCluster checks if the given cluster matches the namespace/name of the management cluster provided on client creation
func (g *Cluster) IsManagementCluster(ctx context.Context, cluster *capi.Cluster) bool {
	return cluster.ObjectMeta.Name == g.managementCluster.Name && cluster.ObjectMeta.Namespace == g.managementCluster.Namespace
}
