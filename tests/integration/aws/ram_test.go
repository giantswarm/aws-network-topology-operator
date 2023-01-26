package aws_test

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
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

		ramClient *aws.RAMClient
	)

	BeforeEach(func() {
		ctx = context.Background()

		name = tests.GenerateGUID("test")
		session, err := session.NewSession(&awssdk.Config{
			Region:   awssdk.String(region),
			Endpoint: awssdk.String(awsEndpoint),
		})
		Expect(err).NotTo(HaveOccurred())

		awsClient := ram.New(session, &awssdk.Config{Credentials: stscreds.NewCredentials(session, awsIAMArn)})
		response, err := awsClient.CreateResourceShareWithContext(ctx, &ram.CreateResourceShareInput{
			AllowExternalPrincipals: awssdk.Bool(true),
			Name:                    awssdk.String(name),
			// Principals:              aws.StringSlice(principals),
			// ResourceArns:            aws.StringSlice(resourceArns),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(response).ToNot(BeNil())

		ramClient = aws.NewRAMClient(awsClient)
	})

	Describe("ApplyResourceShare", func() {
		It("creates the ", func() {
			share := aws.ResourceShare{
				Name:              name,
				ResourceArns:      []string{},
				ExternalAccountID: externalAccount,
			}
			ramClient.ApplyResourceShare(ctx, share)
		})
	})
})
