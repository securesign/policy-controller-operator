//go:build fips

package e2e_fips

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	e2e_utils "github.com/securesign/policy-controller-operator/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/e2e-framework/klient/conf"
)

var (
	k8sClient client.Client
	ctx       context.Context
	scheme    = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func TestFips(t *testing.T) {
	format.MaxLength = 0
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "Policy Controller FIPS E2E Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	kubeconfig := conf.ResolveKubeConfigFile()
	data, err := os.ReadFile(kubeconfig)
	Expect(err).NotTo(HaveOccurred())
	return data
}, func(data []byte) {
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	e2e_utils.SetRestConfig(restCfg)

	ctx = context.Background()

	fmt.Println(">>> Running FIPS E2E tests with the following parameters:")
	fmt.Printf("  %-24s %s\n", "Install Namespace:", e2e_utils.InstallNamespace)
	fmt.Printf("  %-24s %s\n", "Inject CA:", e2e_utils.InjectCA())

	By("ensuring the policy-controller-operator namespace exists")
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: e2e_utils.InstallNamespace},
	})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

	By("applying the PolicyController CR")
	renderedPolicyController, err := e2e_utils.RenderTemplate(e2e_utils.PolicyControllerCRPath, map[string]string{
		"NS": e2e_utils.InstallNamespace,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedPolicyController, "")).To(Succeed())

	By("ensuring deployment is ready")
	Eventually(func(ctx context.Context) error {
		return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
	}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready", e2e_utils.DeploymentName)

	By("detecting and configuring FIPS mode")
	e2e_utils.DetectAndConfigureFIPS(ctx, k8sClient)

	By("injecting CA")
	injectCA, err := strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
	Expect(err).NotTo(HaveOccurred())
	if injectCA {
		Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, e2e_utils.DeploymentName, e2e_utils.InstallNamespace)).To(Succeed())
		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "trusted-ca-bundle", "ca-bundle.crt")
		}).WithContext(ctx).Should(Succeed(), "trusted-ca-bundle never got its ca-bundle.crt")

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready after CA injection", e2e_utils.DeploymentName)
	}

	By("verifying all required resources are created")
	e2e_utils.ExpectRequiredResources(ctx, k8sClient)
})

var _ = AfterSuite(func() {
	By("cleaning up policy controller resources")
	gvk := schema.GroupVersionKind{
		Group:   "rhtas.charts.redhat.com",
		Version: "v1alpha1",
		Kind:    "PolicyController",
	}
	Expect(e2e_utils.DeleteResource(ctx, k8sClient, gvk, "policycontroller-sample", e2e_utils.InstallNamespace)).To(Succeed())
})
