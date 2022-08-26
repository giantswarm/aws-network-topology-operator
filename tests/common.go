package tests

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TestDescription = "test resource for aws-network-topology-operator"
)

func GenerateGUID(prefix string) string {
	guid := uuid.NewString()

	return fmt.Sprintf("%s-%s", prefix, guid[:13])
}

func GetEnvOrSkip(env string) string {
	value := os.Getenv(env)
	if value == "" {
		ginkgo.Skip(fmt.Sprintf("%s not exported", env))
	}

	return value
}

func PatchClusterStatus(k8sClient client.Client, cluster *capi.Cluster, status capi.ClusterStatus) {
	patchedCluster := cluster.DeepCopy()
	patchedCluster.Status = status
	err := k8sClient.Status().Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
	if k8serrors.IsNotFound(err) {
		return
	}

	nsName := types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}
	Expect(k8sClient.Get(context.Background(), nsName, cluster)).To(Succeed())
}

func PatchAWSClusterStatus(k8sClient client.Client, cluster *capa.AWSCluster, status capa.AWSClusterStatus) {
	patchedCluster := cluster.DeepCopy()
	patchedCluster.Status = status
	err := k8sClient.Status().Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
	if k8serrors.IsNotFound(err) {
		return
	}

	nsName := types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}
	Expect(k8sClient.Get(context.Background(), nsName, cluster)).To(Succeed())
}
