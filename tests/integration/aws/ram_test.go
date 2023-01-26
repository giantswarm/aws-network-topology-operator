package aws_test

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ram"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var _ = Describe("RAM", func() {
	var (
		ctx context.Context

		name string

		ec2Client      *ec2.EC2
		transitGateway *ec2.TransitGateway
		ramClient      *aws.RAMClient
	)

	BeforeEach(func() {
		ctx = context.Background()

		name = tests.GenerateGUID("test")
		session, err := session.NewSession(&awssdk.Config{
			Region: awssdk.String(awsRegion),
		})
		Expect(err).NotTo(HaveOccurred())

		ec2Client = ec2.New(session, &awssdk.Config{Credentials: stscreds.NewCredentials(session, iamRoleARN)})
		awsClient := ram.New(session, &awssdk.Config{Credentials: stscreds.NewCredentials(session, iamRoleARN)})
		// response, err := awsClient.CreateResourceShareWithContext(ctx, &ram.CreateResourceShareInput{
		// 	AllowExternalPrincipals: awssdk.Bool(true),
		// 	Name:                    awssdk.String(name),
		// 	// Principals:              aws.StringSlice(principals),
		// 	// ResourceArns:            aws.StringSlice(resourceArns),
		// })

		output, err := ec2Client.CreateTransitGateway(&ec2.CreateTransitGatewayInput{
			Description: awssdk.String("test transit gateway"),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(output).ToNot(BeNil())
		transitGateway = output.TransitGateway
		Expect(transitGateway.TransitGatewayArn).To(Equal("AAA"))

		ramClient = aws.NewRAMClient(awsClient)
	})

	AfterEach(func() {
		_, err := ec2Client.DeleteTransitGateway(&ec2.DeleteTransitGatewayInput{
			TransitGatewayId: transitGateway.TransitGatewayId,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("ApplyResourceShare", func() {
		It("creates the ", func() {
			share := aws.ResourceShare{
				Name:              name,
				ResourceArns:      []string{},
				ExternalAccountID: wcAccount,
			}
			ramClient.ApplyResourceShare(ctx, share)
		})
	})
})
