package aws_test

import (
	"context"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ram"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var _ = Describe("RAM client", func() {
	var (
		ctx context.Context

		name       string
		share      aws.ResourceShare
		prefixList *ec2.ManagedPrefixList

		rawEC2Client *ec2.EC2
		rawRamClient *ram.RAM

		ramClient *aws.RAMClient
	)

	getSharedResources := func(g Gomega) []*ram.Resource {
		listResourcesOutput, err := rawRamClient.ListResources(&ram.ListResourcesInput{
			Principal:     awssdk.String(wcAccount),
			ResourceArns:  awssdk.StringSlice([]string{*prefixList.PrefixListArn}),
			ResourceOwner: awssdk.String(aws.ResourceOwnerSelf),
		})
		g.Expect(err).NotTo(HaveOccurred())
		return listResourcesOutput.Resources
	}

	getResourceAssociationStatus := func(g Gomega) *string {
		listAssociationsOutput, err := rawRamClient.GetResourceShareAssociations(&ram.GetResourceShareAssociationsInput{
			AssociationType: awssdk.String(ram.ResourceShareAssociationTypeResource),
			ResourceArn:     prefixList.PrefixListArn,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(listAssociationsOutput.ResourceShareAssociations).To(HaveLen(1))
		return listAssociationsOutput.ResourceShareAssociations[0].Status
	}

	getPrincipalAssociationStatus := func(g Gomega) *string {
		getResourceShareOutput, err := rawRamClient.GetResourceShares(&ram.GetResourceSharesInput{
			Name:          awssdk.String(name),
			ResourceOwner: awssdk.String(aws.ResourceOwnerSelf),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(getResourceShareOutput.ResourceShares).To(HaveLen(1))

		resourceShare := getResourceShareOutput.ResourceShares[0]
		listAssociationsOutput, err := rawRamClient.GetResourceShareAssociations(&ram.GetResourceShareAssociationsInput{
			AssociationType:   awssdk.String(ram.ResourceShareAssociationTypePrincipal),
			Principal:         awssdk.String(wcAccount),
			ResourceShareArns: []*string{resourceShare.ResourceShareArn},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(listAssociationsOutput.ResourceShareAssociations).To(HaveLen(1))
		return listAssociationsOutput.ResourceShareAssociations[0].Status
	}

	BeforeEach(func() {
		SetDefaultEventuallyTimeout(10 * time.Second)
		SetDefaultConsistentlyPollingInterval(100 * time.Millisecond)

		ctx = log.IntoContext(context.Background(), logger)
		name = tests.GenerateGUID("test")

		session, err := session.NewSession(&awssdk.Config{
			Region: awssdk.String(awsRegion),
		})
		Expect(err).NotTo(HaveOccurred())

		rawEC2Client = ec2.New(session,
			&awssdk.Config{
				Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
			},
		)
		rawRamClient = ram.New(session,
			&awssdk.Config{
				Credentials: stscreds.NewCredentials(session, mcIAMRoleARN),
			},
		)

		prefixList = createManagedPrefixList(rawEC2Client, name)

		share = aws.ResourceShare{
			Name:              name,
			ResourceArns:      []string{*prefixList.PrefixListArn},
			ExternalAccountID: wcAccount,
		}
		ramClient = aws.NewRAMClient(rawRamClient)
	})

	AfterEach(func() {
		_, err := rawEC2Client.DeleteManagedPrefixList(&ec2.DeleteManagedPrefixListInput{PrefixListId: prefixList.PrefixListId})
		Expect(err).NotTo(HaveOccurred())

		getResourceShareOutput, err := rawRamClient.GetResourceShares(&ram.GetResourceSharesInput{
			Name:          awssdk.String(name),
			ResourceOwner: awssdk.String(aws.ResourceOwnerSelf),
		})
		Expect(err).NotTo(HaveOccurred())
		for _, resourceShare := range getResourceShareOutput.ResourceShares {
			_, err = rawRamClient.DeleteResourceShare(&ram.DeleteResourceShareInput{
				ResourceShareArn: resourceShare.ResourceShareArn,
			})
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("ApplyResourceShare", func() {
		It("creates the share resource", func() {
			err := ramClient.ApplyResourceShare(ctx, share)
			Expect(err).NotTo(HaveOccurred())

			Eventually(getSharedResources).Should(HaveLen(1))
		})

		When("the resource has already been shared", func() {
			BeforeEach(func() {
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).NotTo(HaveOccurred())

				Eventually(getSharedResources).Should(HaveLen(1))
			})

			It("does not return an error", func() {
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).NotTo(HaveOccurred())

				Consistently(getSharedResources).Should(HaveLen(1))
			})
		})

		When("when the resource is not owned by the MC account", func() {
			BeforeEach(func() {
				share.ResourceArns = []string{
					"arn:aws:ec2:eu-north-1:012345678901:prefix-list/pl-0123456789abcdeff",
				}
			})

			It("returns an error", func() {
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).To(HaveOccurred())
			})
		})

		When("when the resource arn is invalid", func() {
			BeforeEach(func() {
				share.ResourceArns = []string{
					"this:is:not::an/arn",
				}
			})

			It("returns an error", func() {
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).To(HaveOccurred())
			})
		})

		When("the destination account does invalid", func() {
			BeforeEach(func() {
				share.ExternalAccountID = "notavalidaccount"
			})

			It("returns an error", func() {
				err := ramClient.ApplyResourceShare(ctx, share)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("DeleteResourceShare", func() {
		BeforeEach(func() {
			err := ramClient.ApplyResourceShare(ctx, share)
			Expect(err).NotTo(HaveOccurred())
			Eventually(getSharedResources).Should(HaveLen(1))

			// The resource share can only be deleted when the resource and
			// principal are in a final state like Associated. If we don't
			// wait for this state the tests will flake
			Eventually(getResourceAssociationStatus).Should(PointTo(Equal(ram.ResourceShareAssociationStatusAssociated)))
			Eventually(getPrincipalAssociationStatus).Should(PointTo(Equal(ram.ResourceShareAssociationStatusAssociated)))
		})

		It("deletes the resource share", func() {
			err := ramClient.DeleteResourceShare(ctx, name)
			Expect(err).NotTo(HaveOccurred())

			Eventually(getSharedResources).Should(HaveLen(0))
		})

		When("the resource is already deleted", func() {
			BeforeEach(func() {
				err := ramClient.DeleteResourceShare(ctx, name)
				Expect(err).NotTo(HaveOccurred())

				Eventually(getSharedResources).Should(HaveLen(0))
			})

			It("does not return an error", func() {
				err := ramClient.DeleteResourceShare(ctx, name)
				Expect(err).NotTo(HaveOccurred())
			})

			When("creating a resource share with the same name", func() {
				It("creates the share resource", func() {
					Consistently(getSharedResources).Should(HaveLen(0))

					err := ramClient.ApplyResourceShare(ctx, share)
					Expect(err).NotTo(HaveOccurred())

					Eventually(getSharedResources).Should(HaveLen(1))
				})

				When("the resource has already been recreated", func() {
					BeforeEach(func() {
						err := ramClient.ApplyResourceShare(ctx, share)
						Expect(err).NotTo(HaveOccurred())
					})

					It("does not return an error", func() {
						err := ramClient.ApplyResourceShare(ctx, share)
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})
		})
	})
})
