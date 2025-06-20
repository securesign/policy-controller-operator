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
	installNamespace = "policy-controller-operator"
	testNamespace    = "pco-e2e"

	policyControllerCrPath   = "custom_resources/common_install/policy_controller.yaml.tpl"
	trustRootCrPath          = "custom_resources/common_install/trust_root.yaml.tpl"
	clusterimagepolicyCrPath = "custom_resources/common_install/cluster_image_policy.yaml.tpl"

	deploymentName           = "policycontroller-sample-policy-controller-webhook"
	validatingWebhookName    = "policy.rhtas.com"
	mutatingWebhookName      = "policy.rhtas.com"
	cipValidatingWebhookName = "validating.clusterimagepolicy.rhtas.com"
	cipMutatingWebhookName   = "defaulting.clusterimagepolicy.rhtas.com"
	webhookSvc               = "webhook"
	metricsSvc               = "policycontroller-sample-policy-controller-webhook-metrics"
	secretName               = "webhook-certs"
)

var (
	policyControllerCrABSPath   = ""
	trustRootCrABSPath          = ""
	clusterImagePolicyCrABSPath = ""
	renderedTrustRoot           []byte
	renderedClusteImagePolicy   []byte

	err error
)

var _ = Describe("policy-controller-operator installation", Ordered, func() {

	BeforeAll(func() {
		policyControllerCrABSPath, err = filepath.Abs(policyControllerCrPath)
		Expect(err).ToNot(HaveOccurred())

		trustRootCrABSPath, err = filepath.Abs(trustRootCrPath)
		Expect(err).ToNot(HaveOccurred())

		clusterImagePolicyCrABSPath, err = filepath.Abs(clusterimagepolicyCrPath)
		Expect(err).ToNot(HaveOccurred())

		By("ensuring the policy-controller-operator namespace exists")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: installNamespace},
		})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

		By("applying the operator bundle: " + policyControllerCrABSPath)
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, nil, policyControllerCrABSPath)).To(Succeed())

		DeferCleanup(func() {
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, "test-pod", testNamespace)).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, testNamespace, "")).To(Succeed())
			Expect(e2e_utils.DeleteManifest(ctx, k8sClient, renderedTrustRoot, "")).To(Succeed())
			Expect(e2e_utils.DeleteManifest(ctx, k8sClient, renderedClusteImagePolicy, "")).To(Succeed())
			Expect(e2e_utils.DeleteManifest(ctx, k8sClient, nil, policyControllerCrABSPath)).To(Succeed())
		})
	})

	It("creates all required resources", func() {
		type resource struct {
			name string
			obj  client.Object
		}
		tests := []resource{
			{deploymentName, &appsv1.Deployment{}},
			{validatingWebhookName, &admissionregistrationv1.ValidatingWebhookConfiguration{}},
			{mutatingWebhookName, &admissionregistrationv1.MutatingWebhookConfiguration{}},
			{cipValidatingWebhookName, &admissionregistrationv1.ValidatingWebhookConfiguration{}},
			{cipMutatingWebhookName, &admissionregistrationv1.MutatingWebhookConfiguration{}},
			{webhookSvc, &corev1.Service{}},
			{metricsSvc, &corev1.Service{}},
			{secretName, &corev1.Secret{}},
			{"config-policy-controller", &corev1.ConfigMap{}},
			{"config-image-policies", &corev1.ConfigMap{}},
			{"config-sigstore-keys", &corev1.ConfigMap{}},
			{"policycontroller-sample-policy-controller-webhook-logging", &corev1.ConfigMap{}},
		}
		for _, tt := range tests {
			By("checking " + tt.name)
			e2e_utils.ExpectExists(tt.name, installNamespace, tt.obj, k8sClient, ctx)
		}
	})

	It("eventually has the deployment ready", func() {
		dep := &appsv1.Deployment{}
		e2e_utils.ExpectExists(deploymentName, installNamespace, dep, k8sClient, ctx)

		desired := *dep.Spec.Replicas
		Eventually(func() int32 {
			k8sClient.Get(ctx, client.ObjectKey{Namespace: installNamespace, Name: deploymentName}, dep)
			Expect(err).ToNot(HaveOccurred())
			return dep.Status.ReadyReplicas
		}).Should(Equal(desired), "timed out waiting for %d pods to be Ready in Deployment %q", desired, deploymentName)
	})

	It("injects the CA bundle and the Deployment rolls out", func() {
		inject, err := strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
		if err != nil {
			panic(fmt.Errorf("invalid value for INJECT_CA: %w", err))
		}
		if !inject {
			Skip("CA-injection tests are disabled for this run")
		}

		Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, deploymentName, installNamespace)).To(Succeed())
		Eventually(func() (bool, error) {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: installNamespace, Name: "trusted-ca-bundle"}, cm)
			if err != nil {
				return false, err
			}
			bundle, ok := cm.Data["ca-bundle.crt"]
			return ok && len(bundle) > 0, nil
		}).Should(BeTrue(), "trusted-ca-bundle never got its ca-bundle.crt")

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: installNamespace, Name: deploymentName}, dep)).To(Succeed(), "failed to read Deployment after CA injection")

		desired := *dep.Spec.Replicas
		Eventually(func() (int32, error) {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: installNamespace, Name: deploymentName}, dep); err != nil {
				return 0, err
			}
			return dep.Status.ReadyReplicas, nil
		}).Should(Equal(desired), "timed out waiting for %d Ready replicas in Deployment %q", desired, deploymentName)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func() {
		encodedRoot, err := e2e_utils.ResolveBase64TufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		renderedTrustRoot, err = e2e_utils.RenderTemplate(trustRootCrABSPath, map[string]string{
			"TUFMirror": e2e_utils.TufUrl(),
			"TUFRoot":   encodedRoot,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedTrustRoot, "")).To(Succeed())

		Eventually(func() (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: installNamespace, Name: "config-sigstore-keys"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data["common-install-trust-root"]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the common-install-trust-root key")
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func() {

		renderedClusteImagePolicy, err = e2e_utils.RenderTemplate(clusterImagePolicyCrABSPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          e2e_utils.TestImage(),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func() (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: installNamespace, Name: "config-image-policies"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data["common-install-cluster-image-policy"]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-image-policies' to have the common-install-cluster-image-policy key")
	})

	It("should create a test namespace", func() {
		Expect(e2e_utils.CreateTestNamespace(ctx, k8sClient, testNamespace)).NotTo(HaveOccurred())
	})

	It("should reject pod creation in a watched namespace and sign the image", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, testNamespace)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.VerifyByCosign(ctx, e2e_utils.TestImage())
	})

	It("should reject pod creation in a watched namespace and attach a provenance", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, testNamespace)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachProvenance(ctx, e2e_utils.TestImage())
	})

	It("should reject pod creation in a watched namespace and attach an SBOM", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, testNamespace)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachSBOM(ctx, e2e_utils.TestImage())
	})

	It("should accept the pod", func() {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, testNamespace)).NotTo(HaveOccurred())
	})
})
