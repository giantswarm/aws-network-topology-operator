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

var _ = Describe("RAM client", func() {
	var (
		ctx context.Context

		name string

		rawEC2Client *ec2.EC2
		ramClient    *aws.RAMClient
		rawRamClient *ram.RAM
		prefixList   *ec2.ManagedPrefixList
	)

	BeforeEach(func() {
		ctx = context.Background()

		name = tests.GenerateGUID("test")
		session, err := session.NewSession(&awssdk.Config{
			Region: awssdk.String(awsRegion),
		})
		Expect(err).NotTo(HaveOccurred())

		rawEC2Client = ec2.New(session, &awssdk.Config{Credentials: stscreds.NewCredentials(session, iamRoleARN)})
		rawRamClient = ram.New(session, &awssdk.Config{Credentials: stscreds.NewCredentials(session, iamRoleARN)})
		ramClient = aws.NewRAMClient(rawRamClient)
		createPrefixListOutput, err := rawEC2Client.CreateManagedPrefixList(&ec2.CreateManagedPrefixListInput{
			AddressFamily:  awssdk.String("IPv4"),
			MaxEntries:     awssdk.Int64(2),
			PrefixListName: awssdk.String("topology-operator-e2e"),
		})
		Expect(err).NotTo(HaveOccurred())
		prefixList = createPrefixListOutput.PrefixList
	})

	AfterEach(func() {
		_, err := rawEC2Client.DeleteManagedPrefixList(&ec2.DeleteManagedPrefixListInput{PrefixListId: prefixList.PrefixListId})
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("ApplyResourceShare", func() {
		It("creates the share resource", func() {
			share := aws.ResourceShare{
				Name:              name,
				ResourceArns:      []string{*prefixList.PrefixListArn},
				ExternalAccountID: wcAccount,
			}
			err := ramClient.ApplyResourceShare(ctx, share)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) []*ram.Resource {
				listResourcesOutput, err := rawRamClient.ListResources(&ram.ListResourcesInput{
					Principal:     awssdk.String(wcAccount),
					ResourceArns:  awssdk.StringSlice([]string{*prefixList.PrefixListArn}),
					ResourceOwner: awssdk.String("SELF"),
				})
				g.Expect(err).NotTo(HaveOccurred())
				return listResourcesOutput.Resources
			}).Should(HaveLen(1))
		})

		When("the resource has already been shared", func() {
			var share aws.ResourceShare

			BeforeEach(func() {
				share = aws.ResourceShare{
					Name:              name,
					ResourceArns:      []string{*prefixList.PrefixListArn},
					ExternalAccountID: wcAccount,
				}
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) []*ram.Resource {
					listResourcesOutput, err := rawRamClient.ListResources(&ram.ListResourcesInput{
						Principal:     awssdk.String(wcAccount),
						ResourceArns:  awssdk.StringSlice([]string{*prefixList.PrefixListArn}),
						ResourceOwner: awssdk.String("SELF"),
					})
					g.Expect(err).NotTo(HaveOccurred())
					return listResourcesOutput.Resources
				}).Should(HaveLen(1))
			})

			It("does not return an error", func() {
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).NotTo(HaveOccurred())

				Consistently(func(g Gomega) []*ram.Resource {
					listResourcesOutput, err := rawRamClient.ListResources(&ram.ListResourcesInput{
						Principal:     awssdk.String(wcAccount),
						ResourceArns:  awssdk.StringSlice([]string{*prefixList.PrefixListArn}),
						ResourceOwner: awssdk.String("SELF"),
					})
					g.Expect(err).NotTo(HaveOccurred())
					return listResourcesOutput.Resources
				}, "5s", "500ms").Should(HaveLen(1))
			})
		})
	})
})
