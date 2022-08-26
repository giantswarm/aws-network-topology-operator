package controllers_test

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/giantswarm/aws-network-topology-operator/controllers"
	"github.com/giantswarm/aws-network-topology-operator/controllers/controllersfakes"
	"github.com/giantswarm/aws-network-topology-operator/pkg/aws/awsfakes"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

type ClusterClient interface {
	registrar.ClusterClient
	controllers.ClusterClient
}

var _ = Describe("NewNetworkTopologyReconciler", func() {
	var (
		ctx context.Context

		reconciler    *controllers.NetworkTopologyReconciler
		clusterClient ClusterClient

		transitGatewayID = "abc-123"

		cluster    *capi.Cluster
		awsCluster *capa.AWSCluster

		managementCluster    *capi.Cluster
		managementAWSCluster *capa.AWSCluster

		result       ctrl.Result
		request      ctrl.Request
		reconcileErr error
	)

	BeforeEach(func() {
		logger := zap.New(zap.WriteTo(GinkgoWriter))
		ctx = log.IntoContext(context.Background(), logger)

		mc := types.NamespacedName{
			Name:      "the-mc-name",
			Namespace: namespace,
		}
		clusterClient = k8sclient.NewCluster(k8sClient, mc)

		reconciler = controllers.NewNetworkTopologyReconciler(
			clusterClient,
			[]controllers.Registrar{
				new(controllersfakes.FakeRegistrar),
			},
		)

		{
			awsCluster = &capa.AWSCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "the-cluster",
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: capa.AWSClusterSpec{},
			}
			Expect(k8sClient.Create(ctx, awsCluster)).To(Succeed())
			tests.PatchAWSClusterStatus(k8sClient, awsCluster, capa.AWSClusterStatus{
				Ready: true,
			})

			cluster = &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "the-cluster",
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: capi.ClusterSpec{
					InfrastructureRef: &v1.ObjectReference{
						Kind:       awsCluster.Kind,
						APIVersion: awsCluster.APIVersion,
						Namespace:  awsCluster.ObjectMeta.Namespace,
						Name:       awsCluster.ObjectMeta.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			tests.PatchClusterStatus(k8sClient, cluster, capi.ClusterStatus{
				InfrastructureReady: true,
			})
		}

		{
			managementAWSCluster = &capa.AWSCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        mc.Name,
					Namespace:   mc.Namespace,
					Annotations: map[string]string{},
				},
				Spec: capa.AWSClusterSpec{},
			}
			Expect(k8sClient.Create(ctx, managementAWSCluster)).To(Succeed())
			tests.PatchAWSClusterStatus(k8sClient, managementAWSCluster, capa.AWSClusterStatus{
				Ready: true,
			})

			managementCluster = &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mc.Name,
					Namespace: mc.Namespace,
					Annotations: map[string]string{
						annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
					},
				},
				Spec: capi.ClusterSpec{
					InfrastructureRef: &v1.ObjectReference{
						Kind:       managementAWSCluster.Kind,
						APIVersion: managementAWSCluster.APIVersion,
						Namespace:  managementAWSCluster.ObjectMeta.Namespace,
						Name:       managementAWSCluster.ObjectMeta.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, managementCluster)).To(Succeed())
			tests.PatchClusterStatus(k8sClient, managementCluster, capi.ClusterStatus{
				InfrastructureReady: true,
			})
		}

		request = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "the-cluster",
				Namespace: namespace,
			},
		}
	})

	JustBeforeEach(func() {
		result, reconcileErr = reconciler.Reconcile(ctx, request)
		// _, _ = reconciler.Reconcile(ctx, request)
	})

	It("adds a finalizer to the cluster", func() {
		actualCluster := &capi.Cluster{}
		err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
		Expect(err).NotTo(HaveOccurred())

		Expect(actualCluster.Finalizers).To(ContainElement(controllers.FinalizerNetTop))
	})

	When("the cluster doesn't have the topology mode annotation", func() {
		BeforeEach(func() {

			reconciler = controllers.NewNetworkTopologyReconciler(
				clusterClient,
				[]controllers.Registrar{
					registrar.NewTransitGateway(new(awsfakes.FakeTransitGatewayClient), clusterClient),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				annotations.NetworkTopologyModeAnnotation: "",
			}
			Expect(k8sClient.Patch(ctx, patchedCluster, client.MergeFrom(cluster))).To(Succeed())

			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should default to 'None'", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualAnnotation := actualCluster.Annotations[annotations.NetworkTopologyModeAnnotation]
			Expect(actualAnnotation).To(Equal(annotations.NetworkTopologyModeNone))
		})

		It("should not set the gateway ID", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualID := actualCluster.Annotations[annotations.NetworkTopologyTransitGatewayIDAnnotation]
			Expect(actualID).To(BeEmpty())
		})

		It("does not requeue the event", func() {
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("the cluster topology mode annotation is set to 'None'", func() {
		BeforeEach(func() {
			reconciler = controllers.NewNetworkTopologyReconciler(
				clusterClient,
				[]controllers.Registrar{
					registrar.NewTransitGateway(new(awsfakes.FakeTransitGatewayClient), clusterClient),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeNone,
			}
			Expect(k8sClient.Patch(ctx, patchedCluster, client.MergeFrom(cluster))).To(Succeed())

			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not change the annotation values", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualMode := actualCluster.Annotations[annotations.NetworkTopologyModeAnnotation]
			Expect(actualMode).To(Equal(annotations.NetworkTopologyModeNone))
			actualID := actualCluster.Annotations[annotations.NetworkTopologyTransitGatewayIDAnnotation]
			Expect(actualID).To(BeEmpty())
		})

		It("does not requeue the event", func() {
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("the cluster topology mode annotation is set to 'UserManaged'", func() {
		BeforeEach(func() {
			reconciler = controllers.NewNetworkTopologyReconciler(
				clusterClient,
				[]controllers.Registrar{
					registrar.NewTransitGateway(new(awsfakes.FakeTransitGatewayClient), clusterClient),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeUserManaged,
			}
			Expect(k8sClient.Patch(ctx, patchedCluster, client.MergeFrom(cluster))).To(Succeed())

			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not change the annotation values", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualMode := actualCluster.Annotations[annotations.NetworkTopologyModeAnnotation]
			Expect(actualMode).To(Equal(annotations.NetworkTopologyModeUserManaged))
			actualID := actualCluster.Annotations[annotations.NetworkTopologyTransitGatewayIDAnnotation]
			Expect(actualID).To(BeEmpty())
		})

		It("does not requeue the event", func() {
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("the cluster topology mode annotation is set to 'GiantSwarmManaged'", func() {
		BeforeEach(func() {
			transitGatewayClient := new(awsfakes.FakeTransitGatewayClient)

			transitGatewayClient.DescribeTransitGatewaysReturns(
				&ec2.DescribeTransitGatewaysOutput{
					TransitGateways: []awstypes.TransitGateway{
						{
							TransitGatewayId: &transitGatewayID,
						},
					},
				},
				nil,
			)

			transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
				&ec2.CreateTransitGatewayVpcAttachmentOutput{
					TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
						TransitGatewayAttachmentId: &transitGatewayID,
					},
				},
				nil,
			)

			reconciler = controllers.NewNetworkTopologyReconciler(
				clusterClient,
				[]controllers.Registrar{
					registrar.NewTransitGateway(transitGatewayClient, clusterClient),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeGiantSwarmManaged,
			}
			Expect(k8sClient.Patch(ctx, patchedCluster, client.MergeFrom(cluster))).To(Succeed())

			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not change the mode annotation value", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualMode := actualCluster.Annotations[annotations.NetworkTopologyModeAnnotation]
			Expect(actualMode).To(Equal(annotations.NetworkTopologyModeGiantSwarmManaged))
		})

		It("should set the gateway ID annotation value", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualID := actualCluster.Annotations[annotations.NetworkTopologyTransitGatewayIDAnnotation]
			Expect(actualID).To(Equal(transitGatewayID))
		})

		It("does requeue the event", func() {
			Expect(result.Requeue).To(BeTrue())
			Expect(result.RequeueAfter).ToNot(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

})
