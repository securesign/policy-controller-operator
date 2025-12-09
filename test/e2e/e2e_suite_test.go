package e2e

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
	e2e_utils "github.com/securesign/policy-controller-operator/test/e2e/utils"
	appsv1 "k8s.io/api/apps/v1"
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

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "Policy Controller E2E Suite")

	format.MaxLength = 0
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

	ctx = context.Background()

	fmt.Println(">>> Running tests with the following parameters:")
	fmt.Printf("  %-22s %s\n", "RHTAS Install Namespace:", e2e_utils.RhtasInstallNamespace())
	fmt.Printf("  %-24s %s\n", "TUF URL:", e2e_utils.TufUrl())
	fmt.Printf("  %-24s %s\n", "TSA URL:", e2e_utils.TsaUrl())
	fmt.Printf("  %-24s %s\n", "Rekor URL:", e2e_utils.RekorUrl())
	fmt.Printf("  %-24s %s\n", "Fulcio URL:", e2e_utils.FulcioUrl())
	fmt.Printf("  %-24s %s\n", "OIDC Issuer URL:", e2e_utils.OidcIssuerUrl())
	fmt.Printf("  %-24s %s\n", "OIDC Issuer Subject:", e2e_utils.OidcIssuerSubject())
	fmt.Printf("  %-24s %s\n", "Inject CA:", e2e_utils.InjectCA())

	By("ensuring the policy-controller-operator namespace exists")
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: e2e_utils.InstallNamespace},
	})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

	By("applying the operator bundle: " + policyControllerCRPath)
	renderedPolicyController, err := e2e_utils.RenderTemplate(policyControllerCRPath, map[string]string{
		"NS": e2e_utils.InstallNamespace,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedPolicyController, "")).To(Succeed())

	By("ensuring deployment is ready")
	dep := &appsv1.Deployment{}
	e2e_utils.ExpectExists(e2e_utils.DeploymentName, e2e_utils.InstallNamespace, dep, k8sClient, ctx)
	desired := *dep.Spec.Replicas
	Eventually(func(ctx context.Context) int32 {
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)
		Expect(err).ToNot(HaveOccurred())
		return dep.Status.ReadyReplicas
	}).WithContext(ctx).Should(Equal(desired), "timed out waiting for %d pods to be Ready in Deployment %q", desired, e2e_utils.DeploymentName)

	By("injecting CA")
	injectCA, err = strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
	Expect(err).NotTo(HaveOccurred())
	if injectCA {
		Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, e2e_utils.DeploymentName, e2e_utils.InstallNamespace)).To(Succeed())
		Eventually(func(ctx context.Context) (bool, error) {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "trusted-ca-bundle"}, cm)
			if err != nil {
				return false, err
			}
			bundle, ok := cm.Data["ca-bundle.crt"]
			return ok && len(bundle) > 0, nil
		}).WithContext(ctx).Should(BeTrue(), "trusted-ca-bundle never got its ca-bundle.crt")

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)).To(Succeed(), "failed to read Deployment after CA injection")

		desired := *dep.Spec.Replicas
		Eventually(func(ctx context.Context) (int32, error) {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep); err != nil {
				return 0, err
			}
			return dep.Status.ReadyReplicas, nil
		}).WithContext(ctx).Should(Equal(desired), "timed out waiting for %d Ready replicas in Deployment %q", desired, e2e_utils.DeploymentName)
	}

	By("verifying all required resources are created")
	e2e_utils.ExpectRequiredResources(ctx, k8sClient)

	By("asserting admission webhook behaviour")
	renderedPolicyController, err = e2e_utils.RenderTemplate(policyControllerCRPath, map[string]string{
		"NS": "default",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedPolicyController, "")).
		To(MatchError(ContainSubstring(`PolicyController objects may only be created in the "policy-controller-operator" namespace`)))

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
