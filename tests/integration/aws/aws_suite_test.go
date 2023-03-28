package aws_test

import (
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

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
})

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
