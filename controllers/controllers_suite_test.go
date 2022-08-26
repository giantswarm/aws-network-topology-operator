package controllers_test

import (
	"context"
	"go/build"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubectl/pkg/scheme"
	capa "sigs.k8s.io/cluster-api-provider-aws/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/giantswarm/aws-network-topology-operator/tests"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Suite")
}

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	namespace string
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	tests.GetEnvOrSkip("KUBEBUILDER_ASSETS")

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "sigs.k8s.io", "cluster-api@v1.1.3", "config", "crd", "bases"),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "sigs.k8s.io", "cluster-api-provider-aws@v1.5.0", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = capa.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = capi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if testEnv == nil {
		return
	}
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = BeforeEach(func() {
	namespace = uuid.New().String()
	namespaceObj := &corev1.Namespace{}
	namespaceObj.Name = namespace
	Expect(k8sClient.Create(context.Background(), namespaceObj)).To(Succeed())
})

var _ = AfterEach(func() {
	namespaceObj := &corev1.Namespace{}
	namespaceObj.Name = namespace
	Expect(k8sClient.Delete(context.Background(), namespaceObj)).To(Succeed())
})
