package controllers_test

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/aws"
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

		reconciler           *controllers.NetworkTopologyReconciler
		clusterClient        ClusterClient
		fakeRegistrar        *controllersfakes.FakeRegistrar
		transitGatewayClient *awsfakes.FakeTransitGatewayClient

		transitGatewayID = "abc-123"
		mcVPCId          = "vpc-123"
		wcVPCId          = "vpc-987"

		cluster    *capi.Cluster
		awsCluster *capa.AWSCluster

		managementCluster    *capi.Cluster
		managementAWSCluster *capa.AWSCluster

		result       ctrl.Result
		request      ctrl.Request
		reconcileErr error
	)

	var newCluster = func(name, namespace string, annotations map[string]string, vpcID string) (*capi.Cluster, *capa.AWSCluster) {
		awsCluster := &capa.AWSCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: map[string]string{},
			},
			Spec: capa.AWSClusterSpec{
				NetworkSpec: capa.NetworkSpec{
					VPC: capa.VPCSpec{
						ID: vpcID,
					},
				},
			},
		}
		cluster := &capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: annotations,
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

		Expect(k8sClient.Create(ctx, awsCluster)).To(Succeed())
		tests.PatchAWSClusterStatus(k8sClient, awsCluster, capa.AWSClusterStatus{
			Ready: true,
		})
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		tests.PatchClusterStatus(k8sClient, cluster, capi.ClusterStatus{
			InfrastructureReady: true,
		})

		return cluster, awsCluster
	}

	BeforeEach(func() {
		logger := zap.New(zap.WriteTo(GinkgoWriter))
		ctx = log.IntoContext(context.Background(), logger)

		mc := types.NamespacedName{
			Name:      "the-mc-name",
			Namespace: namespace,
		}
		clusterClient = k8sclient.NewCluster(k8sClient, mc)

		fakeRegistrar = new(controllersfakes.FakeRegistrar)
		reconciler = controllers.NewNetworkTopologyReconciler(
			clusterClient,
			[]controllers.Registrar{
				fakeRegistrar,
			},
		)

		{
			awsCluster = &capa.AWSCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "the-cluster",
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: capa.AWSClusterSpec{
					NetworkSpec: capa.NetworkSpec{
						VPC: capa.VPCSpec{
							ID: wcVPCId,
						},
					},
				},
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
				Spec: capa.AWSClusterSpec{
					NetworkSpec: capa.NetworkSpec{
						VPC: capa.VPCSpec{
							ID: mcVPCId,
						},
					},
				},
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
	})

	It("adds a finalizer to the cluster", func() {
		actualCluster := &capi.Cluster{}
		err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
		Expect(err).NotTo(HaveOccurred())

		Expect(actualCluster.Finalizers).To(ContainElement(controllers.FinalizerNetTop))
	})

	It("uses the registrars to register the records", func() {
		Expect(fakeRegistrar.RegisterCallCount()).To(Equal(1))
		_, actualCluster := fakeRegistrar.RegisterArgsForCall(0)
		Expect(actualCluster.ObjectMeta.UID).To(Equal(cluster.ObjectMeta.UID))
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
			transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

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

		When("the cluster is an Management Cluster", func() {

			When("the cluster is new", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeGiantSwarmManaged,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{},
						},
						nil,
					)

					transitGatewayClient.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayReturns(
						&ec2.CreateTransitGatewayOutput{
							TransitGateway: &awstypes.TransitGateway{
								TransitGatewayId: &transitGatewayID,
								State:            awstypes.TransitGatewayStateAvailable,
							},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &mcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      mcCluster.ObjectMeta.Name,
							Namespace: mcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should create a new transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClient.CreateTransitGatewayArgsForCall(0)
					Expect(payload.TagSpecifications).To(ContainElement(awstypes.TagSpecification{
						ResourceType: awstypes.ResourceTypeTransitGateway,
						Tags: []awstypes.Tag{
							{
								Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", request.Name)),
								Value: aws.String("owned"),
							},
						},
					}))
				})

				It("should create a transit gateway attachment", func() {
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClient.CreateTransitGatewayVpcAttachmentArgsForCall(0)
					Expect(payload.TagSpecifications).To(ContainElement(awstypes.TagSpecification{
						ResourceType: awstypes.ResourceTypeTransitGatewayAttachment,
						Tags: []awstypes.Tag{
							{
								Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", request.Name)),
								Value: aws.String("owned"),
							},
						},
					}))
					Expect(*payload.VpcId).To(Equal(mcVPCId))
				})
			})

			When("the cluster has an existing transit gateway but no attachment", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{
								{
									TransitGatewayId: &transitGatewayID,
									State:            awstypes.TransitGatewayStateAvailable,
								},
							},
						},
						nil,
					)

					transitGatewayClient.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &mcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      mcCluster.ObjectMeta.Name,
							Namespace: mcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should not create a new transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
				})

				It("should create a transit gateway attachment", func() {
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClient.CreateTransitGatewayVpcAttachmentArgsForCall(0)
					Expect(payload.TagSpecifications).To(ContainElement(awstypes.TagSpecification{
						ResourceType: awstypes.ResourceTypeTransitGatewayAttachment,
						Tags: []awstypes.Tag{
							{
								Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", request.Name)),
								Value: aws.String("owned"),
							},
						},
					}))
					Expect(*payload.VpcId).To(Equal(mcVPCId))
				})
			})

			When("the cluster has an existing transit gateway and existing attachment", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{
								{
									TransitGatewayId: &transitGatewayID,
									State:            awstypes.TransitGatewayStateAvailable,
								},
							},
						},
						nil,
					)

					transitGatewayClient.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{
								{
									TransitGatewayId:           &transitGatewayID,
									TransitGatewayAttachmentId: &transitGatewayID,
									VpcId:                      &mcAWSCluster.Spec.NetworkSpec.VPC.ID,
								},
							},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
						nil,
						fmt.Errorf("Conflict"),
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      mcCluster.ObjectMeta.Name,
							Namespace: mcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should not create a new transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
				})

				It("should not create a transit gateway attachment", func() {
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				})
			})

		})

		When("the cluster is a Workload Cluster", func() {
			When("the cluster is new", func() {
				BeforeEach(func() {
					wcCluster, wcAWSCluster := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeGiantSwarmManaged,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{
								{
									TransitGatewayId: &transitGatewayID,
									State:            awstypes.TransitGatewayStateAvailable,
								},
							},
						},
						nil,
					)

					transitGatewayClient.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      wcCluster.ObjectMeta.Name,
							Namespace: wcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should not create a new transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
				})

				It("should create a transit gateway attachment", func() {
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClient.CreateTransitGatewayVpcAttachmentArgsForCall(0)
					Expect(payload.TagSpecifications).To(ContainElement(awstypes.TagSpecification{
						ResourceType: awstypes.ResourceTypeTransitGatewayAttachment,
						Tags: []awstypes.Tag{
							{
								Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", request.Name)),
								Value: aws.String("owned"),
							},
						},
					}))
					Expect(*payload.VpcId).To(Equal(wcVPCId))
				})

			})

			When("the cluster has an existing transit gateway but no attachment", func() {
				BeforeEach(func() {
					wcCluster, wcAWSCluster := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{
								{
									TransitGatewayId: &transitGatewayID,
									State:            awstypes.TransitGatewayStateAvailable,
								},
							},
						},
						nil,
					)

					transitGatewayClient.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      wcCluster.ObjectMeta.Name,
							Namespace: wcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should not create a new transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
				})

				It("should create a transit gateway attachment", func() {
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClient.CreateTransitGatewayVpcAttachmentArgsForCall(0)
					Expect(payload.TagSpecifications).To(ContainElement(awstypes.TagSpecification{
						ResourceType: awstypes.ResourceTypeTransitGatewayAttachment,
						Tags: []awstypes.Tag{
							{
								Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", request.Name)),
								Value: aws.String("owned"),
							},
						},
					}))
					Expect(*payload.VpcId).To(Equal(wcVPCId))
				})
			})

			When("the cluster has an existing transit gateway and existing attachment", func() {
				BeforeEach(func() {
					wcCluster, wcAWSCluster := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{
								{
									TransitGatewayId: &transitGatewayID,
									State:            awstypes.TransitGatewayStateAvailable,
								},
							},
						},
						nil,
					)

					transitGatewayClient.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{
								{
									TransitGatewayId:           &transitGatewayID,
									TransitGatewayAttachmentId: &transitGatewayID,
									VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
								},
							},
						},
						nil,
					)

					transitGatewayClient.CreateTransitGatewayVpcAttachmentReturns(
						nil,
						fmt.Errorf("Conflict"),
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      wcCluster.ObjectMeta.Name,
							Namespace: wcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should not create a new transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
				})

				It("should create a transit gateway attachment", func() {
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				})
			})

			When("the management cluster annotation is set to 'None'", func() {
				BeforeEach(func() {
					wcCluster, _ := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeGiantSwarmManaged,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeNone,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      wcCluster.ObjectMeta.Name,
							Namespace: wcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
					Expect(reconcileErr.Error()).To(ContainSubstring("management cluster doesn't have a TGW specified"))
					Expect(result.Requeue).To(BeTrue())
				})

			})

			When("the a transit gateway matching the ID doesn't exist", func() {
				BeforeEach(func() {
					wcCluster, _ := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation:             annotations.NetworkTopologyModeGiantSwarmManaged,
							annotations.NetworkTopologyTransitGatewayIDAnnotation: "not-exist",
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							annotations.NetworkTopologyModeAnnotation: annotations.NetworkTopologyModeGiantSwarmManaged,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					transitGatewayClient.DescribeTransitGatewaysReturns(
						&ec2.DescribeTransitGatewaysOutput{
							TransitGateways: []awstypes.TransitGateway{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      wcCluster.ObjectMeta.Name,
							Namespace: wcCluster.ObjectMeta.Namespace,
						},
					}
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
					Expect(reconcileErr.Error()).To(ContainSubstring("failed to find TransitGateway for provided ID"))
					Expect(result.Requeue).To(BeTrue())
				})
			})
		})
	})

	When("the cluster does not exist", func() {
		BeforeEach(func() {
			request = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "not-the-cluster",
					Namespace: namespace,
				},
			}
		})

		It("does not requeue the event", func() {
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("the cluster is paused", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			patchedCluster.Spec.Paused = true
			Expect(k8sClient.Patch(ctx, patchedCluster, client.MergeFrom(cluster))).To(Succeed())
		})

		It("does not reconcile", func() {
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})
})
