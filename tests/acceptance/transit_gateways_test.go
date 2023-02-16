package acceptance_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var _ = Describe("Transit Gateways", func() {
	var (
		ctx context.Context

		name                 string
		managementCluster    *capi.Cluster
		managementAWSCluster *capa.AWSCluster
	)

	BeforeEach(func() {
		// SetDefaultEventuallyPollingInterval(time.Second)
		// SetDefaultEventuallyTimeout(time.Second * 90)
		ctx = context.Background()
		name = tests.GenerateGUID("test")
		managementCluster = &capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mc",
				Namespace: "test",
			},
			Spec: capi.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					APIVersion: capa.GroupVersion.String(),
					Kind:       "AWSCluster",
					Namespace:  "test",
					Name:       name,
				},
			},
		}
		managementAWSCluster = &capa.AWSCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "test",
			},
		}

		err := k8sClient.Create(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Create(ctx, managementAWSCluster)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())
		controllerutil.RemoveFinalizer(managementCluster, capa.ClusterFinalizer)
		patchHelper, err := patch.NewHelper(managementCluster, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		err = patchHelper.Patch(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates the transit gateway", func() {
		Expect(true).To(BeFalse())
	})
})
