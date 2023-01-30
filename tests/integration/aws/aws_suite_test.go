package aws_test

import (
	"fmt"
	"testing"

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
