package acceptance_test

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	gsannotation "github.com/giantswarm/k8smetadata/pkg/annotation"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/pkg/k8sclient"
	"github.com/giantswarm/aws-network-topology-operator/pkg/util/annotations"
)

var _ = Describe("Transit Gateways", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		SetDefaultEventuallyPollingInterval(time.Second)
		SetDefaultEventuallyTimeout(5 * time.Minute)

		err := testFixture.Setup()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(testFixture.Teardown()).To(Succeed())
	})

	It("creates the transit gateway", func() {
		By("creating the transit gateway")
		actualCluster := &capi.Cluster{}

		getClusterAnnotations := func(g Gomega) map[string]string {
			nsName := k8sclient.GetNamespacedName(testFixture.ManagementCluster.Cluster)
			err := k8sClient.Get(ctx, nsName, actualCluster)
			g.Expect(err).NotTo(HaveOccurred())

			return actualCluster.Annotations
		}
		Eventually(getClusterAnnotations).Should(HaveKey(gsannotation.NetworkTopologyTransitGatewayIDAnnotation))

		transitGatewayID, err := aws.GetARNResourceID(annotations.GetNetworkTopologyTransitGateway(actualCluster))
		Expect(err).NotTo(HaveOccurred())

		output, err := testFixture.EC2Client.DescribeTransitGateways(&ec2.DescribeTransitGatewaysInput{
			TransitGatewayIds: []*string{awssdk.String(transitGatewayID)},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(output.TransitGateways).To(HaveLen(1))

		By("attaching the transit gateway")
		getTGWAttachments := func() []*ec2.TransitGatewayVpcAttachment {
			describeTGWattachmentInput := &ec2.DescribeTransitGatewayVpcAttachmentsInput{
				Filters: []*ec2.Filter{
					{
						Name:   awssdk.String("transit-gateway-id"),
						Values: []*string{awssdk.String(transitGatewayID)},
					},
					{
						Name:   awssdk.String("vpc-id"),
						Values: []*string{awssdk.String(testFixture.Network.VpcID)},
					},
				},
			}
			describeTGWattachmentOutput, err := testFixture.EC2Client.DescribeTransitGatewayVpcAttachments(describeTGWattachmentInput)
			Expect(err).NotTo(HaveOccurred())
			return describeTGWattachmentOutput.TransitGatewayVpcAttachments
		}
		Eventually(getTGWAttachments).Should(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
			"State": PointTo(Equal("available")),
		}))))

		By("creating the prefix list")
		Eventually(getClusterAnnotations).Should(HaveKey(gsannotation.NetworkTopologyPrefixListIDAnnotation))

		prefixListID, err := aws.GetARNResourceID(annotations.GetNetworkTopologyPrefixList(actualCluster))
		Expect(err).NotTo(HaveOccurred())

		result, err := testFixture.EC2Client.GetManagedPrefixListEntries(&ec2.GetManagedPrefixListEntriesInput{
			PrefixListId: awssdk.String(prefixListID),
			MaxResults:   awssdk.Int64(100),
		})
		Expect(err).NotTo(HaveOccurred())

		managementAWSCluster := testFixture.ManagementCluster.AWSCluster
		prefixListDescription := fmt.Sprintf("CIDR block for cluster %s", managementAWSCluster.Name)
		Expect(result.Entries).To(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
			"Cidr":        PointTo(Equal(managementAWSCluster.Spec.NetworkSpec.VPC.CidrBlock)),
			"Description": PointTo(Equal(prefixListDescription)),
		}))))

		/*
			By("creating a route in explicitly attached route tables")
			getRouteTables := func() []*ec2.RouteTable {
				subnets := []*string{}
				for _, s := range managementAWSCluster.Spec.NetworkSpec.Subnets {
					subnets = append(subnets, awssdk.String(s.ID))
				}

				routeTablesOutput, err := testFixture.EC2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
					Filters: []*ec2.Filter{
						{Name: awssdk.String("association.subnet-id"), Values: subnets},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(routeTablesOutput).NotTo(BeNil())

				return routeTablesOutput.RouteTables
			}
			Eventually(getRouteTables).Should(ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
				"Routes": ContainElement(PointTo(MatchFields(IgnoreExtras, Fields{
					"DestinationPrefixListId": PointTo(Equal(prefixListID)),
					"TransitGatewayId":        PointTo(Equal(transitGatewayID)),
				}))),
			}))))
		*/
	})
})
