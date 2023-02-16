package acceptance_test

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
	"github.com/giantswarm/aws-network-topology-operator/tests"
	gsannotations "github.com/giantswarm/k8smetadata/pkg/annotation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Transit Gateways", func() {
	var (
		ctx context.Context

		name                 string
		managementCluster    *capi.Cluster
		managementAWSCluster *capa.AWSCluster
		clusterIdentity      *capa.AWSClusterRoleIdentity

		rawEC2Client *ec2.EC2
	)

	BeforeEach(func() {
		SetDefaultEventuallyPollingInterval(time.Second)
		SetDefaultEventuallyTimeout(time.Second * 90)
		ctx = context.Background()
		name = tests.GenerateGUID("test")

		clusterIdentity = &capa.AWSClusterRoleIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: capa.AWSClusterRoleIdentitySpec{
				AWSRoleSpec: capa.AWSRoleSpec{
					RoleArn: mcIAMRoleARN,
				},
				SourceIdentityRef: &capa.AWSIdentityReference{
					Name: "default",
					Kind: capa.ControllerIdentityKind,
				},
			},
		}

		err := k8sClient.Create(ctx, clusterIdentity)
		Expect(err).NotTo(HaveOccurred())

		managementCluster = &capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mc",
				Namespace: "test",
				Annotations: map[string]string{
					gsannotations.NetworkTopologyModeAnnotation: gsannotations.NetworkTopologyModeGiantSwarmManaged,
				},
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
				Name:      "test-mc",
				Namespace: "test",
			},
			Spec: capa.AWSClusterSpec{
				IdentityRef: &capa.AWSIdentityReference{
					Name: name,
					Kind: capa.ClusterRoleIdentityKind,
				},
			},
		}

		err = k8sClient.Create(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Create(ctx, managementAWSCluster)
		Expect(err).NotTo(HaveOccurred())

		session, err := session.NewSession(&aws.Config{
			Region: aws.String(awsRegion),
		})
		Expect(err).NotTo(HaveOccurred())

		rawEC2Client = ec2.New(session,
			&aws.Config{
				Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
			},
		)
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
		var transitGatewayID string
		getAnnotation := func() string {
			cluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, k8sclient.GetNamespacedName(managementCluster), cluster)
			Expect(err).NotTo(HaveOccurred())
			transitGatewayID = annotations.GetNetworkTopologyTransitGatewayID(cluster)
			return transitGatewayID
		}

		Eventually(getAnnotation).ShouldNot(BeEmpty())

		output, err := rawEC2Client.DescribeTransitGateways(&ec2.DescribeTransitGatewaysInput{
			TransitGatewayIds: []*string{aws.String(transitGatewayID)},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(output).NotTo(BeNil())
	})
})
