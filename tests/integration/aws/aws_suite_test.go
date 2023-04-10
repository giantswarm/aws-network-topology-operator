package aws_test

import (
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/giantswarm/aws-network-topology-operator/pkg/aws"
	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var (
	logger logr.Logger

	mcAccount string
	wcAccount string
	iamRoleId string
	awsRegion string

	mcIAMRoleARN string
	wcIAMRoleARN string

	rawEC2Client *ec2.EC2
	rawRamClient *ram.RAM
)

func TestAws(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Aws Suite")
}

var _ = BeforeSuite(func() {
	opts := zap.Options{
		DestWriter:  GinkgoWriter,
		Development: true,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}

	logger = zap.New(zap.UseFlagOptions(&opts))

	mcAccount = tests.GetEnvOrSkip("MC_AWS_ACCOUNT")
	wcAccount = tests.GetEnvOrSkip("WC_AWS_ACCOUNT")
	iamRoleId = tests.GetEnvOrSkip("AWS_IAM_ROLE_ID")
	awsRegion = tests.GetEnvOrSkip("AWS_REGION")

	mcIAMRoleARN = fmt.Sprintf("arn:aws:iam::%s:role/%s", mcAccount, iamRoleId)
	wcIAMRoleARN = fmt.Sprintf("arn:aws:iam::%s:role/%s", wcAccount, iamRoleId)

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
})

func getResourceAssociationStatus(resourceShareName string, prefixList *ec2.ManagedPrefixList) func(g Gomega) *string {
	return func(g Gomega) *string {
		getResourceShareOutput, err := rawRamClient.GetResourceShares(&ram.GetResourceSharesInput{
			Name:          awssdk.String(resourceShareName),
			ResourceOwner: awssdk.String(aws.ResourceOwnerSelf),
		})
		Expect(err).NotTo(HaveOccurred())
		resourceShares := []*ram.ResourceShare{}
		for _, share := range getResourceShareOutput.ResourceShares {
			if !isResourceShareDeleted(share) {
				resourceShares = append(resourceShares, share)
			}
		}
		Expect(resourceShares).To(HaveLen(1))

		resourceShare := resourceShares[0]

		listAssociationsOutput, err := rawRamClient.GetResourceShareAssociations(&ram.GetResourceShareAssociationsInput{
			AssociationType:   awssdk.String(ram.ResourceShareAssociationTypeResource),
			ResourceArn:       prefixList.PrefixListArn,
			ResourceShareArns: []*string{resourceShare.ResourceShareArn},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(listAssociationsOutput.ResourceShareAssociations).To(HaveLen(1))
		return listAssociationsOutput.ResourceShareAssociations[0].Status
	}
}

func getPrincipalAssociationStatus(resourceShareName string) func(g Gomega) *string {
	return func(g Gomega) *string {
		getResourceShareOutput, err := rawRamClient.GetResourceShares(&ram.GetResourceSharesInput{
			Name:          awssdk.String(resourceShareName),
			ResourceOwner: awssdk.String(aws.ResourceOwnerSelf),
		})
		Expect(err).NotTo(HaveOccurred())
		resourceShares := []*ram.ResourceShare{}
		for _, share := range getResourceShareOutput.ResourceShares {
			if !isResourceShareDeleted(share) {
				resourceShares = append(resourceShares, share)
			}
		}
		Expect(resourceShares).To(HaveLen(1))

		resourceShare := resourceShares[0]
		listAssociationsOutput, err := rawRamClient.GetResourceShareAssociations(&ram.GetResourceShareAssociationsInput{
			AssociationType:   awssdk.String(ram.ResourceShareAssociationTypePrincipal),
			Principal:         awssdk.String(wcAccount),
			ResourceShareArns: []*string{resourceShare.ResourceShareArn},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(listAssociationsOutput.ResourceShareAssociations).To(HaveLen(1))
		return listAssociationsOutput.ResourceShareAssociations[0].Status
	}
}

func getSharedResources(ramClient *ram.RAM, prefixList *ec2.ManagedPrefixList) func(g Gomega) []*ram.Resource {
	return func(g Gomega) []*ram.Resource {
		listResourcesOutput, err := ramClient.ListResources(&ram.ListResourcesInput{
			Principal:     awssdk.String(wcAccount),
			ResourceArns:  awssdk.StringSlice([]string{*prefixList.PrefixListArn}),
			ResourceOwner: awssdk.String(aws.ResourceOwnerSelf),
		})
		g.Expect(err).NotTo(HaveOccurred())
		return listResourcesOutput.Resources
	}
}

func createManagedPrefixList(ec2Client *ec2.EC2, name string) *ec2.ManagedPrefixList {
	createPrefixListOutput, err := ec2Client.CreateManagedPrefixList(&ec2.CreateManagedPrefixListInput{
		AddressFamily:  awssdk.String("IPv4"),
		MaxEntries:     awssdk.Int64(2),
		PrefixListName: awssdk.String(name),
	})
	Expect(err).NotTo(HaveOccurred())
	prefixList := createPrefixListOutput.PrefixList
	Eventually(func() string {
		prefixListOutput, err := ec2Client.DescribeManagedPrefixLists(&ec2.DescribeManagedPrefixListsInput{
			PrefixListIds: []*string{prefixList.PrefixListId},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(prefixListOutput.PrefixLists).To(HaveLen(1))
		return *prefixListOutput.PrefixLists[0].State
	}).Should(Equal("create-complete"))

	return prefixList
}

func isResourceShareDeleted(resourceShare *ram.ResourceShare) bool {
	if resourceShare.Status == nil {
		return false
	}

	status := *resourceShare.Status
	return status == ram.ResourceShareStatusDeleted || status == ram.ResourceShareStatusDeleting
}
