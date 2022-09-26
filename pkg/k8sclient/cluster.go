package k8sclient

import (
	"context"

	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	capiconditions "sigs.k8s.io/cluster-api/util/conditions"
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
	if err := g.Client.Patch(ctx, capiCluster, client.MergeFrom(originalCluster)); err != nil {
		return err
	}
	capaCluster, err := g.GetAWSCluster(ctx, types.NamespacedName{Name: capiCluster.Name, Namespace: capiCluster.Namespace})
	if err != nil {
		return err
	}
	originalCAPACluster := capaCluster.DeepCopy()
	controllerutil.AddFinalizer(capaCluster, finalizer)
	return g.Client.Patch(ctx, capaCluster, client.MergeFrom(originalCAPACluster))
}

// RemoveFinalizer removes the given finalizer from the cluster
func (g *Cluster) RemoveFinalizer(ctx context.Context, capiCluster *capi.Cluster, finalizer string) error {
	capaCluster, err := g.GetAWSCluster(ctx, types.NamespacedName{Name: capiCluster.Name, Namespace: capiCluster.Namespace})
	// Note: If error is not nil we're going to ignore it and continue removing the finalizer from the CAPI cluster
	if err == nil {
		originalCAPACluster := capaCluster.DeepCopy()
		controllerutil.RemoveFinalizer(capaCluster, finalizer)
		if err := g.Client.Patch(ctx, capaCluster, client.MergeFrom(originalCAPACluster)); err != nil {
			return err
		}
	}
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
	_, found := lookupConditionOrCreateUnknown(ctx, cluster, networkTopologyCondition)
	return found
}

func (g *Cluster) UpdateStatusCondition(ctx context.Context, cluster *capi.Cluster, status corev1.ConditionStatus) error {
	originalCluster := cluster.DeepCopy()
	condition, _ := lookupConditionOrCreateUnknown(ctx, cluster, networkTopologyCondition)
	condition.Status = status
	condition.LastTransitionTime = metav1.Now()

	capiconditions.Set(cluster, condition)
	return g.Client.Status().Patch(ctx, cluster, client.MergeFrom(originalCluster))
}

func lookupConditionOrCreateUnknown(ctx context.Context, cluster *capi.Cluster, conditionType capi.ConditionType) (*capi.Condition, bool) {
	condition := capiconditions.Get(cluster, capi.ConditionType(networkTopologyCondition))

	if condition != nil {
		return condition, true
	}

	return &capi.Condition{
		Type:   capi.ConditionType(conditionType),
		Status: corev1.ConditionUnknown,
	}, false
}
