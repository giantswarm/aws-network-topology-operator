package aws_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var (
	mcAccount  string
	wcAccount  string
	iamRoleARN string
	awsRegion  string
)

func TestAws(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Aws Suite")
}

var _ = BeforeSuite(func() {
	// opts := zap.Options{
	// 	DestWriter:  GinkgoWriter,
	// 	Development: true,
	// 	TimeEncoder: zapcore.RFC3339TimeEncoder,
	// }

	// logger = zap.New(zap.UseFlagOptions(&opts))
	// logf.SetLogger(logger)
	mcAccount = tests.GetEnvOrSkip("MC_AWS_ACCOUNT")
	wcAccount = tests.GetEnvOrSkip("WC_AWS_ACCOUNT")
	iamRoleARN = tests.GetEnvOrSkip("AWS_IAM_ROLE_ARN")
	awsRegion = tests.GetEnvOrSkip("AWS_REGION")
})
