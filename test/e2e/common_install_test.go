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
	policyControllerCommonCrPath   = "custom_resources/policy_controller/common_policy_controller.yaml.tpl"
	trustRootCommonCrPath          = "custom_resources/trust_roots/common_trust_root.yaml.tpl"
	clusterimagepolicyCommonCrPath = "custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"
	commonTestNS                   = "pco-e2e"
	commonTestImageEnv             = "COMMON_TEST_IMAGE"
	commonTrustRootRef             = "common-install-trust-root"
	commonCIPName                  = "common-install-cluster-image-policy"
)

var (
	policyControllerCommonCrABSPath   string
	trustRootCommonCrABSPath          string
	clusterImagePolicyCommonCrABSPath string
	commonTestImage                   string
	commonRenderedPolicyontroller     []byte
	commonRenderedTrustRoot           []byte
	commonRenderedClusteImagePolicy   []byte
)

var _ = Describe("policy-controller-operator common installation", Ordered, func() {
	var err error

	BeforeAll(func(ctx SpecContext) {
		policyControllerCommonCrABSPath, err = filepath.Abs(policyControllerCommonCrPath)
		Expect(err).ToNot(HaveOccurred())

		trustRootCommonCrABSPath, err = filepath.Abs(trustRootCommonCrPath)
		Expect(err).ToNot(HaveOccurred())

		clusterImagePolicyCommonCrABSPath, err = filepath.Abs(clusterimagepolicyCommonCrPath)
		Expect(err).ToNot(HaveOccurred())

		By("ensuring the policy-controller-operator namespace exists")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: e2e_utils.InstallNamespace},
		})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

		By("applying the operator bundle: " + policyControllerCommonCrABSPath)
		commonRenderedPolicyontroller, err = e2e_utils.RenderTemplate(policyControllerCommonCrABSPath, map[string]string{
			"NS": e2e_utils.InstallNamespace,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedPolicyontroller, "")).To(Succeed())

		commonTestImage = e2e_utils.PrepareImage(ctx, commonTestImageEnv)

		DeferCleanup(func(ctx SpecContext) {
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, "test-pod", commonTestNS)).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, commonTestNS, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, commonCIPName, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, commonTrustRootRef, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "rhtas.charts.redhat.com", Version: "v1alpha1", Kind: "PolicyController"}, "policycontroller-sample", e2e_utils.InstallNamespace)).To(Succeed())
		})
	})

	It("creates all required resources", func(ctx SpecContext) {
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

	It("eventually has the deployment ready", func(ctx SpecContext) {
		dep := &appsv1.Deployment{}
		e2e_utils.ExpectExists(e2e_utils.DeploymentName, e2e_utils.InstallNamespace, dep, k8sClient, ctx)

		desired := *dep.Spec.Replicas
		Eventually(func(ctx SpecContext) int32 {
			k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)
			Expect(err).ToNot(HaveOccurred())
			return dep.Status.ReadyReplicas
		}).WithContext(ctx).Should(Equal(desired), "timed out waiting for %d pods to be Ready in Deployment %q", desired, e2e_utils.DeploymentName)
	})

	It("injects the CA bundle and the Deployment rolls out", func(ctx SpecContext) {
		inject, err := strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
		if err != nil {
			panic(fmt.Errorf("invalid value for INJECT_CA: %w", err))
		}
		if !inject {
			Skip("CA-injection tests are disabled for this run")
		}

		Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, e2e_utils.DeploymentName, e2e_utils.InstallNamespace)).To(Succeed())
		Eventually(func(ctx SpecContext) (bool, error) {
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
		Eventually(func(ctx SpecContext) (int32, error) {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep); err != nil {
				return 0, err
			}
			return dep.Status.ReadyReplicas, nil
		}).WithContext(ctx).Should(Equal(desired), "timed out waiting for %d Ready replicas in Deployment %q", desired, e2e_utils.DeploymentName)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		tufroot, err := e2e_utils.ResolveTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		commonRenderedTrustRoot, err = e2e_utils.RenderTemplate(trustRootCommonCrABSPath, map[string]string{
			"TUFMirror": e2e_utils.TufUrl(),
			"TUFRoot":   e2e_utils.Base64EncodeString(tufroot),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedTrustRoot, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-sigstore-keys"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[commonTrustRootRef]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", commonTrustRootRef)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {

		commonRenderedClusteImagePolicy, err = e2e_utils.RenderTemplate(clusterImagePolicyCommonCrABSPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          commonTestImage,
			"TRUST_ROOT_REF":      commonTrustRootRef,
			"CIP_NAME":            commonCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-image-policies"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[commonCIPName]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", commonCIPName)
	})

	It("should create a test namespace", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestNamespace(ctx, k8sClient, commonTestNS)).NotTo(HaveOccurred())

		Eventually(func(g Gomega, ctx SpecContext) {
			ns := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: commonTestNS}, ns)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ns.Status.Phase).To(Equal(corev1.NamespaceActive))
		}).WithContext(ctx).Should(Succeed())
	})

	It("should reject controller creation in wrong namespace", func(ctx SpecContext) {
		commonRenderedPolicyontroller, err = e2e_utils.RenderTemplate(policyControllerCommonCrABSPath, map[string]string{
			"NS": commonTestNS,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedPolicyontroller, "")).
			To(MatchError(ContainSubstring(`PolicyController objects may only be created in the "policy-controller-operator" namespace`)))
	})

	It("should reject pod creation in a watched namespace and sign the image", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, commonTestNS, commonTestImage)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.VerifyByCosign(ctx, commonTestImage)
	})

	It("should reject pod creation in a watched namespace and attach a provenance", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, commonTestNS, commonTestImage)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachProvenance(ctx, commonTestImage)
	})

	It("should reject pod creation in a watched namespace and attach an SBOM", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, commonTestNS, commonTestImage)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachSBOM(ctx, commonTestImage)
	})

	It("should accept the pod", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, commonTestNS, commonTestImage)).NotTo(HaveOccurred())
	})
})
