package k8sclient

import (
	"context"
	"time"

	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var networkTopologyCondition capi.ConditionType = "NetworkTopologyReady"

type Cluster struct {
	Client            client.Client
	managementCluster types.NamespacedName
}

// NewCluster returns a new Cluster client
func NewCluster(client client.Client, managementCluster types.NamespacedName) *Cluster {
	return &Cluster{
		Client:            client,
		managementCluster: managementCluster,
	}
}

// Get retrieves a Cluster based on the given namespace/name
func (g *Cluster) Get(ctx context.Context, namespacedName types.NamespacedName) (*capi.Cluster, error) {
	cluster := &capi.Cluster{}
	err := g.Client.Get(ctx, namespacedName, cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// GetManagementCluster retrieves the Cluster for the management cluster namespace/name provided at client creation
func (g *Cluster) GetManagementCluster(ctx context.Context) (*capi.Cluster, error) {
	cluster := &capi.Cluster{}
	err := g.Client.Get(ctx, g.managementCluster, cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

func (g *Cluster) GetManagementClusterNamespacedName() types.NamespacedName {
	return g.managementCluster
}

// GetAWSCluster retrieves an AWSCluster based on the provided namespace/name
func (g *Cluster) GetAWSCluster(ctx context.Context, namespacedName types.NamespacedName) (*capa.AWSCluster, error) {
	cluster := &capa.AWSCluster{}
	err := g.Client.Get(ctx, namespacedName, cluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// Patch applies the given patches to the cluster
func (g *Cluster) Patch(ctx context.Context, cluster *capi.Cluster, patch client.Patch) (*capi.Cluster, error) {
	err := g.Client.Patch(ctx, cluster, patch, &client.PatchOptions{})
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return cluster, microerror.Mask(err)
}

// AddFinalizer adds the given finalizer to the cluster
func (g *Cluster) AddFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	originalCluster := capiCluster.DeepCopy()
	controllerutil.AddFinalizer(capiCluster, finalizer)
	return g.Client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster))
}

// RemoveFinalizer removes the given finalizer from the cluster
func (g *Cluster) RemoveFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	originalCluster := capiCluster.DeepCopy()
	controllerutil.RemoveFinalizer(capiCluster, finalizer)
	return g.Client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster))
}

// IsManagementCluster checks if the given cluster matches the namespace/name of the management cluster provided on client creation
func (g *Cluster) IsManagementCluster(ctx context.Context, cluster *capi.Cluster) bool {
	return cluster.ObjectMeta.Name == g.managementCluster.Name && cluster.ObjectMeta.Namespace == g.managementCluster.Namespace
}

// GetAWSCluster retrieves an AWSCluster based on the provided namespace/name
func (g *Cluster) GetAWSClusterRoleIdentity(ctx context.Context, namespacedName types.NamespacedName) (*capa.AWSClusterRoleIdentity, error) {
	identity := &capa.AWSClusterRoleIdentity{}

	c, err := g.GetAWSCluster(ctx, namespacedName)
	if err != nil {
		return nil, err
	}

	err = g.Client.Get(ctx, types.NamespacedName{Name: c.Spec.IdentityRef.Name}, identity)
	if err != nil {
		return nil, microerror.Mask(err)
	}
	return identity, microerror.Mask(err)
}

func (g *Cluster) HasStatusCondition(ctx context.Context, cluster *capi.Cluster) bool {
	for _, c := range cluster.Status.Conditions {
		if c.Type == networkTopologyCondition {
			return true
		}
	}
	return false
}

func (g *Cluster) UpdateStatusCondition(ctx context.Context, cluster *capi.Cluster, status corev1.ConditionStatus) error {
	originalCluster := cluster.DeepCopy()

	found := false
	condition := capi.Condition{
		Type:               networkTopologyCondition,
		Status:             status,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	for _, c := range cluster.Status.Conditions {
		if c.Type == networkTopologyCondition {
			found = true
			c.LastTransitionTime = condition.LastTransitionTime
			c.Status = condition.Status
		}
	}

	if !found {
		cluster.Status.Conditions = append(cluster.Status.Conditions, condition)
	}

	return g.Client.Status().Patch(ctx, cluster, client.MergeFrom(originalCluster))
}
