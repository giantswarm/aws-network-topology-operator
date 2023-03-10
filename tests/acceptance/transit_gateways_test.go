package acceptance_test

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ram"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/giantswarm/aws-network-topology-operator/controllers"
	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
	"github.com/giantswarm/aws-network-topology-operator/tests/acceptance"
)

var _ = Describe("Transit Gateways", func() {
	var (
		ctx               context.Context
		fixture           *acceptance.Fixture
		prefixListID      string
		prefixListARN     string
		rawEC2Client      *ec2.EC2
		transitGatewayARN string
		ramClient         *ram.RAM
	)

	BeforeEach(func() {
		ctx = context.Background()
		SetDefaultEventuallyPollingInterval(time.Second)
		SetDefaultEventuallyTimeout(5 * time.Minute)
		session, err := session.NewSession(&awssdk.Config{
			Region: awssdk.String(awsRegion),
		})
		Expect(err).NotTo(HaveOccurred())

		rawEC2Client = ec2.New(session,
			&awssdk.Config{
				Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
			},
		)

		ramClient = ram.New(session, &awssdk.Config{
			Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
		})

		fixture = &acceptance.Fixture{}
		err = fixture.Setup(ctx, k8sClient, rawEC2Client, mcIAMRoleARN, awsRegion, availabilityZone)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		managementCluster := fixture.GetManagementCluster()
		managementAWSCluster := fixture.GetManagementAWSCluster()
		clusterRoleIdentity := fixture.GetClusterRoleIdentity()

		workloadCluster := fixture.GetWorkloadCluster()
		workloadAWSCluster := fixture.GetWorkloadAWSCluster()
		workloadClusterRoleIdentity := fixture.GetWorkloadClusterRoleIdentity()

		err := k8sClient.Delete(ctx, workloadCluster)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) []string {
			err := k8sClient.Get(ctx, k8sclient.GetNamespacedName(workloadCluster), workloadCluster)
			if k8serrors.IsNotFound(err) {
				return nil
			}
			g.Expect(err).NotTo(HaveOccurred())
			return workloadCluster.Finalizers
		}).ShouldNot(ContainElement(controllers.FinalizerNetTop))

		err = k8sClient.Delete(ctx, workloadAWSCluster)
		if !k8serrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		err = k8sClient.Delete(ctx, workloadClusterRoleIdentity)
		if !k8serrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		controllerutil.RemoveFinalizer(workloadCluster, capa.ClusterFinalizer)
		patchHelper, err := patch.NewHelper(workloadCluster, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		err = patchHelper.Patch(ctx, workloadCluster)
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Delete(ctx, managementCluster)
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

		err = k8sClient.Delete(ctx, clusterRoleIdentity)
		if !k8serrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		controllerutil.RemoveFinalizer(managementCluster, capa.ClusterFinalizer)
		patchHelper, err = patch.NewHelper(managementCluster, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		err = patchHelper.Patch(ctx, managementCluster)
		Expect(err).NotTo(HaveOccurred())

		err = fixture.Teardown(ctx, k8sClient, rawEC2Client)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates the transit gateway", func() {
		var transitGatewayID string
		getTransitGatewayID := func(g Gomega) string {
			cluster := &capi.Cluster{}
			err := k8sClient.Get(ctx, fixture.GetManagementClusterNamespacedName(), cluster)
			g.Expect(err).NotTo(HaveOccurred())
			transitGatewayARN = annotations.GetNetworkTopologyTransitGateway(cluster)
			if transitGatewayARN == "" {
				return ""
			}
			transitGatewayID, err = aws.GetARNResourceID(transitGatewayARN)
			g.Expect(err).NotTo(HaveOccurred())
			return transitGatewayID
		}

		Eventually(getTransitGatewayID).ShouldNot(BeEmpty())
		output, err := rawEC2Client.DescribeTransitGateways(&ec2.DescribeTransitGatewaysInput{
			TransitGatewayIds: []*string{awssdk.String(transitGatewayID)},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(output).NotTo(BeNil())

		getTGWAttachments := func() []*ec2.TransitGatewayVpcAttachment {
			describeTGWattachmentInput := &ec2.DescribeTransitGatewayVpcAttachmentsInput{
				Filters: []*ec2.Filter{
					{
						Name:   awssdk.String("transit-gateway-id"),
						Values: []*string{awssdk.String(transitGatewayID)},
					},
					{
						Name:   awssdk.String("vpc-id"),
						Values: []*string{awssdk.String(fixture.GetVpcID())},
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
			prefixListARN = annotations.GetNetworkTopologyPrefixList(cluster)
			if prefixListARN == "" {
				return ""
			}

			prefixListID, err = aws.GetARNResourceID(prefixListARN)
			Expect(err).NotTo(HaveOccurred())

			return prefixListID
		}
		Eventually(getPrefixlistIDAnnotation).ShouldNot(BeEmpty())

		result, err := rawEC2Client.GetManagedPrefixListEntries(&ec2.GetManagedPrefixListEntriesInput{
			PrefixListId: awssdk.String(prefixListID),
			MaxResults:   awssdk.Int64(100),
		})
		Expect(err).NotTo(HaveOccurred())

		managementAWSCluster := fixture.GetManagementAWSCluster()
		prefixListDescription := fmt.Sprintf("CIDR block for cluster %s", managementAWSCluster.Name)
		Expect(result.Entries).To(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
			"Cidr":        PointTo(Equal(managementAWSCluster.Spec.NetworkSpec.VPC.CidrBlock)),
			"Description": PointTo(Equal(prefixListDescription)),
		}))))

		checkRouteTables := func() []*ec2.RouteTable {
			subnets := []*string{}
			for _, s := range managementAWSCluster.Spec.NetworkSpec.Subnets {
				subnets = append(subnets, awssdk.String(s.ID))
			}

			routeTablesOutput, err := rawEC2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
				Filters: []*ec2.Filter{
					{Name: awssdk.String("association.subnet-id"), Values: subnets},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(routeTablesOutput).NotTo(BeNil())

			return routeTablesOutput.RouteTables
		}
		Eventually(checkRouteTables).Should(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
			"Routes": ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
				"DestinationPrefixListId": PointTo(Equal(prefixListID)),
				"TransitGatewayId":        PointTo(Equal(transitGatewayID)),
			}))),
		}))))

		err = fixture.CreateWCOnAnotherAccount(ctx, k8sClient, rawEC2Client, wcIAMRoleARN, awsRegion, availabilityZone, transitGatewayARN, prefixListARN)
		Expect(err).NotTo(HaveOccurred())

		getResourceShares := func() []*ram.ResourceShare {
			resourceShare, err := ramClient.GetResourceShares(&ram.GetResourceSharesInput{
				Name:          awssdk.String(fmt.Sprintf("%s-transit-gateway", fixture.GetWorkloadClusterNamespacedName().Name)),
				ResourceOwner: awssdk.String("SELF"),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceShare).NotTo(BeNil())
			return resourceShare.ResourceShares
		}
		Eventually(getResourceShares).Should(HaveLen(1))
	})
})
