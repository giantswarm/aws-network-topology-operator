package acceptance_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/aws-network-topology-operator/tests"
)

var (
	k8sClient client.Client

	namespace         string
	namespaceObj      *corev1.Namespace
	mcIAMRoleARN      string
	awsRegion         string
	wcIAMRoleARN      string
	externalAccountID string
	availabilityZone  string
)

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Acceptance Suite")
}

var _ = BeforeSuite(func() {
	tests.GetEnvOrSkip("KUBECONFIG")

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	err = capa.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = capi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	mcAccount := tests.GetEnvOrSkip("MC_AWS_ACCOUNT")
	wcAccount := tests.GetEnvOrSkip("WC_AWS_ACCOUNT")
	//wcOtherAccount := tests.GetEnvOrSkip("WC_AWS_ACCOUNT_ORIGINAL")
	iamRoleId := tests.GetEnvOrSkip("AWS_IAM_ROLE_ID")
	awsRegion = tests.GetEnvOrSkip("AWS_REGION")
	availabilityZone = fmt.Sprintf("%sa", awsRegion)
	mcIAMRoleARN = fmt.Sprintf("arn:aws:iam::%s:role/%s", mcAccount, iamRoleId)
	wcIAMRoleARN = fmt.Sprintf("arn:aws:iam::%s:role/%s", wcAccount, iamRoleId)
	externalAccountID = tests.GetEnvOrSkip("MC_AWS_ACCOUNT")
})

var _ = BeforeEach(func() {
	namespace = uuid.New().String()
	namespaceObj = &corev1.Namespace{}
	namespaceObj.Name = namespace
	Expect(k8sClient.Create(context.Background(), namespaceObj)).To(Succeed())
})

var _ = AfterEach(func() {
	Expect(k8sClient.Delete(context.Background(), namespaceObj)).To(Succeed())
})
