package controllers_test

import (
	"context"
	"errors"
	"fmt"

	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/giantswarm/aws-network-topology-operator/controllers"
	"github.com/giantswarm/aws-network-topology-operator/controllers/controllersfakes"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var _ = Describe("Share", func() {
	var (
		ctx context.Context

		name              string
		sourceAccountID   = "123456789012"
		transitGatewayARN = fmt.Sprintf("arn:aws:ec2:eu-west-2:%s:transit-gateway/tgw-01234567890abcdef", sourceAccountID)
		prefixListARN     = fmt.Sprintf("arn:aws:ec2:eu-west-2:%s:prefix-list/pl-01234567890abcdef", sourceAccountID)
		externalAccountID = "987654321098"
		notValidArn       = "not:a:valid/arn"

		cluster         *capi.Cluster
		clusterIdentity *capa.AWSClusterRoleIdentity
		awsCluster      *capa.AWSCluster
		request         ctrl.Request

		ramClient  *controllersfakes.FakeRAMClient
		reconciler *controllers.ShareReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()

		name = tests.GenerateGUID("test")
		cluster = &capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Annotations: map[string]string{
					gsannotation.NetworkTopologyTransitGatewayIDAnnotation: transitGatewayARN,
					gsannotation.NetworkTopologyPrefixListIDAnnotation:     prefixListARN,
					gsannotation.NetworkTopologyModeAnnotation:             gsannotation.NetworkTopologyModeGiantSwarmManaged,
				},
			},
			Spec: capi.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					Kind:      "AWSCluster",
					Namespace: namespace,
					Name:      name,
				},
			},
		}
		err := k8sClient.Create(ctx, cluster)
		Expect(err).NotTo(HaveOccurred())

		clusterIdentity = &capa.AWSClusterRoleIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: capa.AWSClusterRoleIdentitySpec{
				AWSRoleSpec: capa.AWSRoleSpec{
					RoleArn: fmt.Sprintf("arn:aws:iam::%s:role/the-role-name", externalAccountID),
				},
			},
		}
		err = k8sClient.Create(ctx, clusterIdentity)
		Expect(err).NotTo(HaveOccurred())

		awsCluster = &capa.AWSCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: capa.AWSClusterSpec{
				IdentityRef: &capa.AWSIdentityReference{
					Kind: "AWSClusterRoleIdentity",
					Name: name,
				},
			},
		}
		err = k8sClient.Create(ctx, awsCluster)
		Expect(err).NotTo(HaveOccurred())

		request = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}

		ramClient = new(controllersfakes.FakeRAMClient)
		reconciler = controllers.NewShareReconciler(
			k8sclient.NewCluster(k8sClient, types.NamespacedName{}),
			ramClient,
		)
	})

	It("shares the transit gateway", func() {
		result, err := reconciler.Reconcile(ctx, request)

		Expect(result.Requeue).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())

		Expect(ramClient.ApplyResourceShareCallCount()).To(Equal(2))
		_, resourceShare := ramClient.ApplyResourceShareArgsForCall(0)
		Expect(resourceShare.Name).To(Equal(fmt.Sprintf("%s-transit-gateway", name)))
		Expect(resourceShare.ResourceArns).To(ConsistOf(transitGatewayARN))
		Expect(resourceShare.ExternalAccountID).To(Equal(externalAccountID))
		_, resourceShare = ramClient.ApplyResourceShareArgsForCall(1)
		Expect(resourceShare.Name).To(Equal(fmt.Sprintf("%s-prefix-list", name)))
		Expect(resourceShare.ResourceArns).To(ConsistOf(prefixListARN))
		Expect(resourceShare.ExternalAccountID).To(Equal(externalAccountID))
	})

	It("adds a finalizer", func() {
		result, err := reconciler.Reconcile(ctx, request)
		Expect(result.Requeue).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())

		actualCluster := &capa.AWSCluster{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: awsCluster.Name, Namespace: awsCluster.Namespace}, actualCluster)
		Expect(err).NotTo(HaveOccurred())

		Expect(actualCluster.Finalizers).To(ContainElement(controllers.FinalizerResourceShare))
	})

	When("adding the finalizer fails", func() {
		BeforeEach(func() {
			fakeClusterClient := new(controllersfakes.FakeClusterClient)
			fakeClusterClient.GetReturns(cluster, nil)
			fakeClusterClient.GetAWSClusterRoleIdentityReturns(clusterIdentity, nil)
			fakeClusterClient.AddFinalizerReturns(errors.New("boom"))
			reconciler = controllers.NewShareReconciler(fakeClusterClient, ramClient)
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(MatchError(ContainSubstring("boom")))
		})
	})

	When("the cluster has been deleted", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			controllerutil.AddFinalizer(patchedCluster, controllers.FinalizerResourceShare)
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			err := k8sClient.Delete(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes the resource share", func() {
			result, err := reconciler.Reconcile(ctx, request)
			Expect(result.Requeue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			Expect(ramClient.DeleteResourceShareCallCount()).To(Equal(2))
			_, actualNameTransitGateway := ramClient.DeleteResourceShareArgsForCall(0)
			Expect(actualNameTransitGateway).To(Equal(fmt.Sprintf("%s-transit-gateway", name)))
			_, actualNamePrefixList := ramClient.DeleteResourceShareArgsForCall(1)
			Expect(actualNamePrefixList).To(Equal(fmt.Sprintf("%s-prefix-list", name)))
		})

		It("removes the finalizer", func() {
			result, err := reconciler.Reconcile(ctx, request)
			Expect(result.Requeue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, k8sclient.GetNamespacedName(cluster), &capi.Cluster{})
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("deleting the resource share fails", func() {
			BeforeEach(func() {
				ramClient.DeleteResourceShareReturns(errors.New("boom"))
			})

			It("retuns an error", func() {
				_, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(MatchError(ContainSubstring("boom")))

				actualCluster := &capi.Cluster{}
				err = k8sClient.Get(ctx, k8sclient.GetNamespacedName(cluster), actualCluster)
				Expect(err).NotTo(HaveOccurred())

				Expect(actualCluster.Finalizers).To(ContainElement(controllers.FinalizerResourceShare))
			})
		})

		When("removing the finalizer fails", func() {
			BeforeEach(func() {
				now := metav1.Now()
				cluster.DeletionTimestamp = &now
				fakeClusterClient := new(controllersfakes.FakeClusterClient)
				fakeClusterClient.GetReturns(cluster, nil)
				fakeClusterClient.RemoveFinalizerReturns(errors.New("boom"))
				reconciler = controllers.NewShareReconciler(fakeClusterClient, ramClient)
			})

			It("returns an error", func() {
				_, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("the cluster still has the networktopology finalizer", func() {
			BeforeEach(func() {
				patchedCluster := cluster.DeepCopy()
				controllerutil.AddFinalizer(patchedCluster, controllers.FinalizerNetTop)
				err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not reconcile", func() {
				result, err := reconciler.Reconcile(ctx, request)

				Expect(result.Requeue).To(BeFalse())
				Expect(err).NotTo(HaveOccurred())

				Expect(ramClient.DeleteResourceShareCallCount()).To(Equal(0))
			})
		})
	})

	When("the transit gateway hasn't been created yet", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			patchedCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation] = ""
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not reconcile", func() {
			result, err := reconciler.Reconcile(ctx, request)

			Expect(result.Requeue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			Expect(ramClient.ApplyResourceShareCallCount()).To(Equal(1))
		})
	})

	When("the transit gateway hasn't been created yet", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			patchedCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation] = ""
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
			Expect(err).NotTo(HaveOccurred())
		})

		It("still shares the prefix list", func() {
			result, err := reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			Expect(ramClient.ApplyResourceShareCallCount()).To(Equal(1))
			_, resourceShare := ramClient.ApplyResourceShareArgsForCall(0)
			Expect(resourceShare.ResourceArns).To(ConsistOf(prefixListARN))
		})
	})

	When("the transit gateway is in the same account as the cluster", func() {
		BeforeEach(func() {
			patchedIdentity := clusterIdentity.DeepCopy()
			patchedIdentity.Spec.RoleArn = fmt.Sprintf("arn:aws:iam::%s:role/the-role-name", sourceAccountID)
			err := k8sClient.Patch(context.Background(), patchedIdentity, client.MergeFrom(clusterIdentity))
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not reconcile", func() {
			result, err := reconciler.Reconcile(ctx, request)

			Expect(result.Requeue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			Expect(ramClient.ApplyResourceShareCallCount()).To(Equal(0))
		})
	})

	When("applying the resource share fails", func() {
		BeforeEach(func() {
			ramClient.ApplyResourceShareReturns(errors.New("boom"))
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(MatchError(ContainSubstring("boom")))
		})
	})

	When("the cluster no longer exists", func() {
		BeforeEach(func() {
			request.Name = "does-not-exist"
		})

		It("does not reconcile", func() {
			result, err := reconciler.Reconcile(ctx, request)

			Expect(result.Requeue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			Expect(ramClient.ApplyResourceShareCallCount()).To(Equal(0))
		})
	})

	When("getting the cluster returns an error", func() {
		BeforeEach(func() {
			fakeClusterClient := new(controllersfakes.FakeClusterClient)
			fakeClusterClient.GetReturns(nil, errors.New("boom"))
			reconciler = controllers.NewShareReconciler(fakeClusterClient, ramClient)
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(MatchError(ContainSubstring("boom")))
		})
	})

	When("the transit gateway arn annotation is invalid", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			patchedCluster.Annotations[gsannotation.NetworkTopologyTransitGatewayIDAnnotation] = notValidArn
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
		})
	})

	When("the prefix list arn annotation is invalid", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			patchedCluster.Annotations[gsannotation.NetworkTopologyPrefixListIDAnnotation] = notValidArn
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
		})
	})

	When("the AWSCluster does not exist", func() {
		BeforeEach(func() {
			patchedCluster := cluster.DeepCopy()
			patchedCluster.Spec.InfrastructureRef = &corev1.ObjectReference{
				Kind:      "AWSCluster",
				Namespace: namespace,
				Name:      "does-not-exist",
			}
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(cluster))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
		})
	})

	When("the AWSClusterRoleIdentity does not exist", func() {
		BeforeEach(func() {
			patchedCluster := awsCluster.DeepCopy()
			patchedCluster.Spec.IdentityRef = &capa.AWSIdentityReference{
				Kind: "AWSClusterRoleIdentity",
				Name: "does-not-exist",
			}
			err := k8sClient.Patch(context.Background(), patchedCluster, client.MergeFrom(awsCluster))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
		})
	})

	When("the AWSClusterRoleIdentity ARN is invalid", func() {
		BeforeEach(func() {
			patchedIdentity := clusterIdentity.DeepCopy()
			patchedIdentity.Spec.RoleArn = notValidArn
			err := k8sClient.Patch(context.Background(), patchedIdentity, client.MergeFrom(clusterIdentity))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error", func() {
			_, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
		})
	})
})
