package e2e

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e_utils "github.com/securesign/policy-controller-operator/test/e2e/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	policyControllerBYOKCrPath   = "custom_resources/byok/policy_controller.yaml.tpl"
	trustRootBYOKCrPath          = "custom_resources/byok/trust_root.yaml.tpl"
	clusterimagepolicyBYOKCrPath = "custom_resources/byok/cluster_image_policy.yaml.tpl"
	byokTestNS                   = "pco-e2e-byok"
	byokTestImageEnv             = "BYOK_TEST_IMAGE"
)

var (
	policyControllerBYOKCrABSPath   = ""
	trustRootBYOKCrABSPath          = ""
	clusterImagePolicyBYOKCrABSPath = ""
	byokRenderedTrustRoot           []byte
	byokRenderedClusteImagePolicy   []byte
)

var _ = Describe("policy-controller-operator byok", Ordered, func() {
	var err error

	BeforeAll(func() {
		policyControllerBYOKCrABSPath, err = filepath.Abs(policyControllerBYOKCrPath)
		Expect(err).ToNot(HaveOccurred())

		trustRootBYOKCrABSPath, err = filepath.Abs(trustRootBYOKCrPath)
		Expect(err).ToNot(HaveOccurred())

		clusterImagePolicyBYOKCrABSPath, err = filepath.Abs(clusterimagepolicyBYOKCrPath)
		Expect(err).ToNot(HaveOccurred())

		By("ensuring the policy-controller-operator namespace exists")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: e2e_utils.InstallNamespace},
		})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

		By("applying the operator bundle: " + policyControllerBYOKCrABSPath)
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, nil, policyControllerBYOKCrABSPath)).To(Succeed())

		DeferCleanup(func() {
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, "test-pod", byokTestNS)).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, byokTestNS, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, "byok-install-trust-root", "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, "byok-install-cluster-image-policy", "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "rhtas.charts.redhat.com", Version: "v1alpha1", Kind: "PolicyController"}, "policycontroller-sample", e2e_utils.InstallNamespace)).To(Succeed())
		})
	})

	It("creates all required resources", func() {
		type resource struct {
			name string
			obj  client.Object
		}
		tests := []resource{
			{e2e_utils.DeploymentName, &appsv1.Deployment{}},
			{e2e_utils.ValidatingWebhookName, &admissionregistrationv1.ValidatingWebhookConfiguration{}},
			{e2e_utils.MutatingWebhookName, &admissionregistrationv1.MutatingWebhookConfiguration{}},
			{e2e_utils.CipValidatingWebhookName, &admissionregistrationv1.ValidatingWebhookConfiguration{}},
			{e2e_utils.CipMutatingWebhookName, &admissionregistrationv1.MutatingWebhookConfiguration{}},
			{e2e_utils.WebhookSvc, &corev1.Service{}},
			{e2e_utils.MetricsSvc, &corev1.Service{}},
			{e2e_utils.SecretName, &corev1.Secret{}},
			{"config-policy-controller", &corev1.ConfigMap{}},
			{"config-image-policies", &corev1.ConfigMap{}},
			{"config-sigstore-keys", &corev1.ConfigMap{}},
			{"policycontroller-sample-policy-controller-webhook-logging", &corev1.ConfigMap{}},
		}
		for _, tt := range tests {
			By("checking " + tt.name)
			e2e_utils.ExpectExists(tt.name, e2e_utils.InstallNamespace, tt.obj, k8sClient, ctx)
		}
	})

	It("eventually has the deployment ready", func() {
		dep := &appsv1.Deployment{}
		e2e_utils.ExpectExists(e2e_utils.DeploymentName, e2e_utils.InstallNamespace, dep, k8sClient, ctx)

		desired := *dep.Spec.Replicas
		Eventually(func() int32 {
			k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)
			Expect(err).ToNot(HaveOccurred())
			return dep.Status.ReadyReplicas
		}).Should(Equal(desired), "timed out waiting for %d pods to be Ready in Deployment %q", desired, e2e_utils.DeploymentName)
	})

	It("injects the CA bundle and the Deployment rolls out", func() {
		inject, err := strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
		if err != nil {
			panic(fmt.Errorf("invalid value for INJECT_CA: %w", err))
		}
		if !inject {
			Skip("CA-injection tests are disabled for this run")
		}

		Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, e2e_utils.DeploymentName, e2e_utils.InstallNamespace)).To(Succeed())
		Eventually(func() (bool, error) {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "trusted-ca-bundle"}, cm)
			if err != nil {
				return false, err
			}
			bundle, ok := cm.Data["ca-bundle.crt"]
			return ok && len(bundle) > 0, nil
		}).Should(BeTrue(), "trusted-ca-bundle never got its ca-bundle.crt")

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)).To(Succeed(), "failed to read Deployment after CA injection")

		desired := *dep.Spec.Replicas
		Eventually(func() (int32, error) {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep); err != nil {
				return 0, err
			}
			return dep.Status.ReadyReplicas, nil
		}).Should(Equal(desired), "timed out waiting for %d Ready replicas in Deployment %q", desired, e2e_utils.DeploymentName)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func() {
		trustedrootValues, err := e2e_utils.ParseTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		byokRenderedTrustRoot, err = e2e_utils.RenderTemplate(trustRootBYOKCrABSPath, map[string]string{
			"FULCIO_ORG_NAME":      trustedrootValues.FulcioOrgName,
			"FULCIO_COMMON_NAME":   trustedrootValues.FulcioCommonName,
			"FULCIO_URL":           e2e_utils.FulcioUrl(),
			"FULCIO_CERT_CHAIN":    trustedrootValues.FulcioCertChain,
			"CTLOG_URL":            fmt.Sprintf("http://ctlog.%s.svc.cluster.local", e2e_utils.RhtasInstallNamespace()),
			"CTLOG_HASH_ALGORITHM": trustedrootValues.CtLogHashAlgo,
			"CTFE_PUBLIC_KEY":      trustedrootValues.CtfePublicKey,
			"REKOR_URL":            e2e_utils.RekorUrl(),
			"REKOR_HASH_ALGORITHM": trustedrootValues.RekorHashAlgo,
			"REKOR_PUBLIC_KEY":     trustedrootValues.RekorPublicKey,
			"TSA_ORG_NAME":         trustedrootValues.TsaOrgName,
			"TSA_COMMON_NAME":      trustedrootValues.TsaCommonName,
			"TSA_URL":              e2e_utils.TsaUrl(),
			"TSA_CERT_CHAIN":       trustedrootValues.TsaCertChain,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, byokRenderedTrustRoot, "")).To(Succeed())

		Eventually(func() (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-sigstore-keys"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data["byok-install-trust-root"]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the byok-install-trust-root key")
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func() {

		byokRenderedClusteImagePolicy, err = e2e_utils.RenderTemplate(clusterImagePolicyBYOKCrABSPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          e2e_utils.PrepareImage(ctx, byokTestImageEnv),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, byokRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func() (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-image-policies"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data["byok-install-cluster-image-policy"]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-image-policies' to have the byok-install-cluster-image-policy key")
	})

	It("should create a test namespace", func() {
		Expect(e2e_utils.CreateTestNamespace(ctx, k8sClient, byokTestNS)).NotTo(HaveOccurred())
	})

	It("should reject pod creation in a watched namespace and sign the image", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, byokTestNS, e2e_utils.PrepareImage(ctx, byokTestImageEnv))).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.VerifyByCosign(ctx, e2e_utils.PrepareImage(ctx, byokTestImageEnv))
	})

	It("should reject pod creation in a watched namespace and attach a provenance", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, byokTestNS, e2e_utils.PrepareImage(ctx, byokTestImageEnv))).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachProvenance(ctx, e2e_utils.PrepareImage(ctx, byokTestImageEnv))
	})

	It("should reject pod creation in a watched namespace and attach an SBOM", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, byokTestNS, e2e_utils.PrepareImage(ctx, byokTestImageEnv))).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachSBOM(ctx, e2e_utils.PrepareImage(ctx, byokTestImageEnv))
	})

	It("should accept the pod", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, byokTestNS, e2e_utils.PrepareImage(ctx, byokTestImageEnv))).NotTo(HaveOccurred())
	})
})
