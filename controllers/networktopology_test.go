package controllers_test

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/aws"
	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"
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
	awsclient "github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/aws/awsfakes"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/registrar"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

type ClusterClient interface {
	registrar.ClusterClient
	controllers.ClusterClient
}

var _ = Describe("NewNetworkTopologyReconciler", func() {
	var (
		ctx context.Context

		reconciler                             *controllers.NetworkTopologyReconciler
		clusterClient                          ClusterClient
		fakeRegistrar                          *controllersfakes.FakeRegistrar
		transitGatewayClient                   *awsfakes.FakeTransitGatewayClient
		transitGatewayClientForWorkloadCluster *awsfakes.FakeTransitGatewayClient

		transitGatewayID = "abc-123"
		prefixListID     = "prefix-123"
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
					Subnets: capa.Subnets{
						{
							ID:       "sub-1",
							IsPublic: false,
						},
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
						Subnets: capa.Subnets{
							{
								ID:       "sub-1",
								IsPublic: false,
							},
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
						Subnets: capa.Subnets{
							{
								ID:       "sub-1",
								IsPublic: false,
							},
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
						gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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
					registrar.NewTransitGateway(new(awsfakes.FakeTransitGatewayClient), clusterClient, nil),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				gsannotation.NetworkTopologyModeAnnotation: "",
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

			actualAnnotation := actualCluster.Annotations[gsannotation.NetworkTopologyModeAnnotation]
			Expect(actualAnnotation).To(Equal(gsannotation.NetworkTopologyModeNone))
		})

		It("should not set the gateway ID", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualID := actualCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation]
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
					registrar.NewTransitGateway(new(awsfakes.FakeTransitGatewayClient), clusterClient, nil),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeNone,
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

			actualMode := actualCluster.Annotations[gsannotation.NetworkTopologyModeAnnotation]
			Expect(actualMode).To(Equal(gsannotation.NetworkTopologyModeNone))
			actualID := actualCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation]
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

			transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
			transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
				&ec2.CreateTransitGatewayVpcAttachmentOutput{
					TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
						TransitGatewayAttachmentId: &transitGatewayID,
					},
				},
				nil,
			)
			getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
				panic("Should not be called in this test case")
			}

			reconciler = controllers.NewNetworkTopologyReconciler(
				clusterClient,
				[]controllers.Registrar{
					registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeUserManaged,
				gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
				gsannotation.NetworkTopologyPrefixListIDAnnotation:     prefixListID,
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

			actualMode := actualCluster.Annotations[gsannotation.NetworkTopologyModeAnnotation]
			Expect(actualMode).To(Equal(gsannotation.NetworkTopologyModeUserManaged))

			actualID := actualCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation]
			Expect(actualID).To(Equal(transitGatewayID))
		})

		It("does requeue the event", func() {
			Expect(result.Requeue).To(BeTrue())
			Expect(result.RequeueAfter).ToNot(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		When("the cluster is a Management Cluster", func() {
			BeforeEach(func() {
				mcCluster, wcAWSCluster := newCluster(
					fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
					map[string]string{
						gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeUserManaged,
						gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						gsannotation.NetworkTopologyPrefixListIDAnnotation:     prefixListID,
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

				transitGatewayClient.DescribeRouteTablesReturns(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []awstypes.RouteTable{
							{
								RouteTableId: aws.String("rt-123"),
								Routes:       []awstypes.Route{},
							},
						},
					},
					nil,
				)

				clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
					Name:      mcCluster.ObjectMeta.Name,
					Namespace: mcCluster.ObjectMeta.Namespace,
				})

				transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
				transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
					&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
						TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
					},
					nil,
				)
				transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
					&ec2.CreateTransitGatewayVpcAttachmentOutput{
						TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
							TransitGatewayAttachmentId: &transitGatewayID,
							TransitGatewayId:           &transitGatewayID,
							VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
							State:                      awstypes.TransitGatewayAttachmentStatePending,
						},
					},
					nil,
				)
				getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
					Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
					return transitGatewayClientForWorkloadCluster
				}

				reconciler = controllers.NewNetworkTopologyReconciler(
					clusterClient,
					[]controllers.Registrar{
						registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
				// Management vs. workload cluster AWS account
				Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

				_, payload, _ := transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentArgsForCall(0)
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

			It("should send the SNS message", func() {
				Expect(transitGatewayClient.PublishSNSMessageCallCount()).To(Equal(1))
			})

			It("should create routes on subnet route tables", func() {
				Expect(transitGatewayClient.CreateRouteCallCount()).To(Equal(1))
			})

		})

		When("the cluster is a Workload Cluster", func() {
			BeforeEach(func() {
				wcCluster, wcAWSCluster := newCluster(
					fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
					map[string]string{
						gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeUserManaged,
						gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						gsannotation.NetworkTopologyPrefixListIDAnnotation:     prefixListID,
					},
					wcVPCId,
				)

				mcCluster, _ := newCluster(
					fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
					map[string]string{
						gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeUserManaged,
						gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						gsannotation.NetworkTopologyPrefixListIDAnnotation:     prefixListID,
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

				transitGatewayClient.DescribeRouteTablesReturns(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []awstypes.RouteTable{
							{
								RouteTableId: aws.String("rt-123"),
								Routes:       []awstypes.Route{},
							},
						},
					},
					nil,
				)

				clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
					Name:      mcCluster.ObjectMeta.Name,
					Namespace: mcCluster.ObjectMeta.Namespace,
				})

				transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
				transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
					&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
						TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
					},
					nil,
				)
				transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
					&ec2.CreateTransitGatewayVpcAttachmentOutput{
						TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
							TransitGatewayAttachmentId: &transitGatewayID,
							TransitGatewayId:           &transitGatewayID,
							VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
							State:                      awstypes.TransitGatewayAttachmentStatePending,
						},
					},
					nil,
				)
				getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
					Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
					return transitGatewayClientForWorkloadCluster
				}

				reconciler = controllers.NewNetworkTopologyReconciler(
					clusterClient,
					[]controllers.Registrar{
						registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
				// Management vs. workload cluster AWS account
				Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

				_, payload, _ := transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentArgsForCall(0)
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

			It("should send the SNS message", func() {
				Expect(transitGatewayClient.PublishSNSMessageCallCount()).To(Equal(1))
			})

			It("should create routes on subnet route tables", func() {
				Expect(transitGatewayClient.CreateRouteCallCount()).To(Equal(1))
			})

		})

		When("the TGW attachment is active", func() {
			BeforeEach(func() {
				mcCluster, wcAWSCluster := newCluster(
					fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
					map[string]string{
						gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeUserManaged,
						gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						gsannotation.NetworkTopologyPrefixListIDAnnotation:     prefixListID,
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

				transitGatewayClient.DescribeRouteTablesReturns(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []awstypes.RouteTable{
							{
								RouteTableId: aws.String("rt-123"),
								Routes:       []awstypes.Route{},
							},
						},
					},
					nil,
				)

				clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
					Name:      mcCluster.ObjectMeta.Name,
					Namespace: mcCluster.ObjectMeta.Namespace,
				})

				transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
				transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
					&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
						TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{
							{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
								State:                      awstypes.TransitGatewayAttachmentStateAvailable,
							},
						},
					},
					nil,
				)
				getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
					Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
					return transitGatewayClientForWorkloadCluster
				}

				reconciler = controllers.NewNetworkTopologyReconciler(
					clusterClient,
					[]controllers.Registrar{
						registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
				// Management vs. workload cluster AWS account
				Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
			})

			It("should not send the SNS message", func() {
				Expect(transitGatewayClient.PublishSNSMessageCallCount()).To(Equal(0))
			})

			It("should create routes on subnet route tables", func() {
				Expect(transitGatewayClient.CreateRouteCallCount()).To(Equal(1))
			})

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

			transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
			transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
				&ec2.CreateTransitGatewayVpcAttachmentOutput{
					TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
						TransitGatewayAttachmentId: &transitGatewayID,
					},
				},
				nil,
			)
			getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
				panic("Should not be called in this test case")
			}

			reconciler = controllers.NewNetworkTopologyReconciler(
				clusterClient,
				[]controllers.Registrar{
					registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
				},
			)

			patchedCluster := cluster.DeepCopy()
			patchedCluster.Finalizers = []string{controllers.FinalizerNetTop}
			patchedCluster.Annotations = map[string]string{
				gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeGiantSwarmManaged,
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

			actualMode := actualCluster.Annotations[gsannotation.NetworkTopologyModeAnnotation]
			Expect(actualMode).To(Equal(gsannotation.NetworkTopologyModeGiantSwarmManaged))
		})

		It("should set the gateway ID annotation value", func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			actualID := actualCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation]
			Expect(actualID).To(Equal(transitGatewayID))
		})

		It("does requeue the event", func() {
			Expect(result.Requeue).To(BeTrue())
			Expect(result.RequeueAfter).ToNot(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		When("the cluster is a Management Cluster", func() {

			When("the cluster is new", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeGiantSwarmManaged,
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

					transitGatewayClient.CreateTransitGatewayReturns(
						&ec2.CreateTransitGatewayOutput{
							TransitGateway: &awstypes.TransitGateway{
								TransitGatewayId: &transitGatewayID,
								State:            awstypes.TransitGatewayStateAvailable,
							},
						},
						nil,
					)

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{},
						nil,
					)

					transitGatewayClient.CreateManagedPrefixListReturns(
						&ec2.CreateManagedPrefixListOutput{
							PrefixList: &awstypes.ManagedPrefixList{
								PrefixListId: &prefixListID,
								Version:      aws.Int64(1),
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)
					transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &mcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(mcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentArgsForCall(0)
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

				It("should not create routes on subnet route tables", func() {
					Expect(transitGatewayClient.CreateRouteCallCount()).To(Equal(0))
				})
			})

			When("the cluster has an existing transit gateway but no attachment", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)
					transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &mcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(mcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentArgsForCall(0)
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

				It("should not create routes on subnet route tables", func() {
					Expect(transitGatewayClient.CreateRouteCallCount()).To(Equal(0))
				})
			})

			When("the cluster has an existing transit gateway and existing attachment", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
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
					transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
						nil,
						fmt.Errorf("Conflict"),
					)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(mcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				})

				It("should not create routes on subnet route tables", func() {
					Expect(transitGatewayClient.CreateRouteCallCount()).To(Equal(0))
				})
			})

			When("the cluster with existing transit gateway attachment gets deleted", func() {
				BeforeEach(func() {
					mcCluster, mcAWSCluster := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
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
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(mcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      mcCluster.ObjectMeta.Name,
							Namespace: mcCluster.ObjectMeta.Namespace,
						},
					}

					// Creation
					_, reconcileErr = reconciler.Reconcile(ctx, request)
					Expect(reconcileErr).NotTo(HaveOccurred())

					Expect(k8sClient.Delete(ctx, mcCluster)).To(Succeed())
				})

				It("detach the transit gateway attachment but not the management cluster's transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
					Expect(transitGatewayClient.DeleteTransitGatewayCallCount()).To(Equal(0))

					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.DeleteTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.DeleteTransitGatewayVpcAttachmentCallCount()).To(Equal(1))
				})
			})
		})

		When("the cluster is a Workload Cluster", func() {
			When("the cluster is new", func() {
				BeforeEach(func() {
					wcCluster, wcAWSCluster := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeGiantSwarmManaged,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)
					transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentArgsForCall(0)
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
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
						&ec2.DescribeTransitGatewayVpcAttachmentsOutput{
							TransitGatewayVpcAttachments: []awstypes.TransitGatewayVpcAttachment{},
						},
						nil,
					)
					transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
						&ec2.CreateTransitGatewayVpcAttachmentOutput{
							TransitGatewayVpcAttachment: &awstypes.TransitGatewayVpcAttachment{
								TransitGatewayAttachmentId: &transitGatewayID,
								TransitGatewayId:           &transitGatewayID,
								VpcId:                      &wcAWSCluster.Spec.NetworkSpec.VPC.ID,
							},
						},
						nil,
					)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(1))

					_, payload, _ := transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentArgsForCall(0)
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
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
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
					transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentReturns(
						nil,
						fmt.Errorf("Conflict"),
					)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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

				It("should not create a transit gateway attachment", func() {
					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.CreateTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
				})
			})

			When("the management cluster annotation is set to 'None'", func() {
				BeforeEach(func() {
					wcCluster, _ := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeGiantSwarmManaged,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeNone,
						},
						mcVPCId,
					)

					transitGatewayClient = new(awsfakes.FakeTransitGatewayClient)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(wcCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: "not-exist",
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeGiantSwarmManaged,
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

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(wcCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
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

			When("the cluster with existing transit gateway attachment gets deleted", func() {
				BeforeEach(func() {
					wcCluster, wcAWSCluster := newCluster(
						fmt.Sprintf("wc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation: gsannotation.NetworkTopologyModeGiantSwarmManaged,
						},
						wcVPCId,
					)

					mcCluster, _ := newCluster(
						fmt.Sprintf("mc-cluster-%d", GinkgoParallelProcess()), namespace,
						map[string]string{
							gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
							gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayID,
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

					transitGatewayClient.DescribeManagedPrefixListsReturns(
						&ec2.DescribeManagedPrefixListsOutput{
							PrefixLists: []awstypes.ManagedPrefixList{
								{PrefixListId: &prefixListID, Version: aws.Int64(1)},
							},
						},
						nil,
					)

					transitGatewayClient.GetManagedPrefixListEntriesReturns(
						&ec2.GetManagedPrefixListEntriesOutput{
							Entries: []awstypes.PrefixListEntry{},
						},
						nil,
					)

					clusterClient = k8sclient.NewCluster(k8sClient, types.NamespacedName{
						Name:      mcCluster.ObjectMeta.Name,
						Namespace: mcCluster.ObjectMeta.Namespace,
					})

					transitGatewayClientForWorkloadCluster = new(awsfakes.FakeTransitGatewayClient)
					transitGatewayClientForWorkloadCluster.DescribeTransitGatewayVpcAttachmentsReturns(
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
					getTransitGatewayClientForWorkloadCluster := func(workloadCluster types.NamespacedName) awsclient.TransitGatewayClient {
						Expect(workloadCluster.Name).To((Equal(wcAWSCluster.Name)))
						return transitGatewayClientForWorkloadCluster
					}

					reconciler = controllers.NewNetworkTopologyReconciler(
						clusterClient,
						[]controllers.Registrar{
							registrar.NewTransitGateway(transitGatewayClient, clusterClient, getTransitGatewayClientForWorkloadCluster),
						},
					)

					request = ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      wcCluster.ObjectMeta.Name,
							Namespace: wcCluster.ObjectMeta.Namespace,
						},
					}

					// Creation
					_, reconcileErr = reconciler.Reconcile(ctx, request)
					Expect(reconcileErr).NotTo(HaveOccurred())

					Expect(k8sClient.Delete(ctx, wcCluster)).To(Succeed())
				})

				It("detach the transit gateway attachment but not the management cluster's transit gateway", func() {
					Expect(transitGatewayClient.CreateTransitGatewayCallCount()).To(Equal(0))
					Expect(transitGatewayClient.DeleteTransitGatewayCallCount()).To(Equal(0))

					// Management vs. workload cluster AWS account
					Expect(transitGatewayClient.DeleteTransitGatewayVpcAttachmentCallCount()).To(Equal(0))
					Expect(transitGatewayClientForWorkloadCluster.DeleteTransitGatewayVpcAttachmentCallCount()).To(Equal(1))
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

	When("the cluster is deleted and doesn't have our network topology finalizer", func() {
		BeforeEach(func() {
			actualCluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, request.NamespacedName, actualCluster)
			Expect(err).NotTo(HaveOccurred())

			Expect(clusterClient.RemoveFinalizer(ctx, actualCluster, controllers.FinalizerNetTop)).To(Succeed())
			Expect(actualCluster.Finalizers).NotTo(ContainElement(controllers.FinalizerNetTop))

			Expect(clusterClient.AddFinalizer(ctx, actualCluster, "testing")).To(Succeed())

			Expect(k8sClient.Delete(ctx, actualCluster)).To(Succeed())
		})

		FIt("does not requeue the event", func() {
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

	})
})
