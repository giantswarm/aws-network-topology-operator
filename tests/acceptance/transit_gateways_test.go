package acceptance_test

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/giantswarm/aws-network-topology-operator/controllers"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
	"github.com/giantswarm/aws-network-topology-operator/tests/acceptance"
)

var _ = Describe("Transit Gateways", func() {
	var (
		ctx              context.Context
		fixture          *acceptance.Fixture
		transitGatewayID string
		prefixListID     string
		rawEC2Client     *ec2.EC2
	)

	BeforeEach(func() {
		ctx = context.Background()
		SetDefaultEventuallyPollingInterval(time.Second)
		SetDefaultEventuallyTimeout(5 * time.Minute)
		session, err := session.NewSession(&aws.Config{
			Region: aws.String(awsRegion),
		})
		Expect(err).NotTo(HaveOccurred())

		rawEC2Client = ec2.New(session,
			&aws.Config{
				Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
			},
		)

		fixture = &acceptance.Fixture{}
		err = fixture.Setup(ctx, k8sClient, rawEC2Client, mcIAMRoleARN, awsRegion, availabilityZone)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		managementCluster := fixture.GetManagementCluster()
		managementAWSCluster := fixture.GetManagementAWSCluster()
		clusterIdentity := fixture.GetClusterRoleIdentity()

		err := k8sClient.Delete(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) []string {
			err := k8sClient.Get(ctx, k8sclient.GetNamespacedName(managementCluster), managementCluster)
			if k8serrors.IsNotFound(err) {
				return nil
			}
			g.Expect(err).NotTo(HaveOccurred())
			return managementCluster.Finalizers
		}).ShouldNot(ContainElement(controllers.FinalizerNetTop))

		err = k8sClient.Delete(ctx, managementAWSCluster)
		if !k8serrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		err = k8sClient.Delete(ctx, clusterIdentity)
		if !k8serrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		controllerutil.RemoveFinalizer(managementCluster, capa.ClusterFinalizer)
		patchHelper, err := patch.NewHelper(managementCluster, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		err = patchHelper.Patch(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())

		err = fixture.Teardown(ctx, k8sClient, rawEC2Client)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates the transit gateway", func() {
		getAnnotation := func() string {
			cluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, fixture.GetManagementClusterNamespacedName(), cluster)
			Expect(err).NotTo(HaveOccurred())
			transitGatewayID = annotations.GetNetworkTopologyTransitGateway(cluster)
			return transitGatewayID
		}

		Eventually(getAnnotation).ShouldNot(BeEmpty())
		output, err := rawEC2Client.DescribeTransitGateways(&ec2.DescribeTransitGatewaysInput{
			TransitGatewayIds: []*string{aws.String(transitGatewayID)},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(output).NotTo(BeNil())

		getTGWAttachments := func() []*ec2.TransitGatewayVpcAttachment {
			describeTGWattachmentInput := &ec2.DescribeTransitGatewayVpcAttachmentsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("transit-gateway-id"),
						Values: []*string{aws.String(transitGatewayID)},
					},
					{
						Name:   aws.String("vpc-id"),
						Values: []*string{aws.String(fixture.GetVpcID())},
					},
				},
			}
			describeTGWattachmentOutput, err := rawEC2Client.DescribeTransitGatewayVpcAttachments(describeTGWattachmentInput)
			Expect(err).NotTo(HaveOccurred())
			return describeTGWattachmentOutput.TransitGatewayVpcAttachments
		}
		Eventually(getTGWAttachments).ShouldNot(HaveLen(0))

		getPrefixlistIDAnnotation := func() string {
			cluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, fixture.GetManagementClusterNamespacedName(), cluster)
			Expect(err).NotTo(HaveOccurred())
			prefixListID = annotations.GetNetworkTopologyPrefixListID(cluster)
			return prefixListID
		}
		Eventually(getPrefixlistIDAnnotation).ShouldNot(BeEmpty())

		result, err := rawEC2Client.GetManagedPrefixListEntries(&ec2.GetManagedPrefixListEntriesInput{
			PrefixListId: aws.String(prefixListID),
			MaxResults:   aws.Int64(100),
		})
		Expect(err).NotTo(HaveOccurred())

		prefixListDescription := fmt.Sprintf("CIDR block for cluster %s", fixture.GetManagementAWSCluster().ObjectMeta.Name)
		descriptionFound := false
		for _, entry := range result.Entries {
			if *entry.Cidr == fixture.GetManagementAWSCluster().Spec.NetworkSpec.VPC.CidrBlock {
				if *entry.Description == prefixListDescription {
					descriptionFound = true
				}
				break
			}
		}
		Expect(descriptionFound).To(BeTrue())
		checkRouteTables := func() []*ec2.RouteTable {
			subnets := []*string{}
			for _, s := range fixture.GetManagementAWSCluster().Spec.NetworkSpec.Subnets {
				subnets = append(subnets, aws.String(s.ID))
			}

			routeTablesOutput, err := rawEC2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
				Filters: []*ec2.Filter{
					{Name: aws.String("association.subnet-id"), Values: subnets},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(routeTablesOutput).NotTo(BeNil())
			Expect(routeTablesOutput.RouteTables).NotTo(HaveLen(0))

			return routeTablesOutput.RouteTables
		}
		Eventually(checkRouteTables).Should(ContainElement(MatchFields(IgnoreExtras, Fields{
			"Routes": ContainElement(MatchFields(IgnoreExtras, Fields{
				"DestinationPrefixListId": Equal(prefixListID),
				"TransitGatewayId":        Equal(transitGatewayID),
			})),
		})))
	})
})
