package e2e

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e_utils "github.com/securesign/policy-controller-operator/test/e2e/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	policyControllerSTUFCrPath   = "custom_resources/policy_controller/common_policy_controller.yaml.tpl"
	trustRootSTUFCrPath          = "custom_resources/trust_roots/stuf_trust_root.yaml.tpl"
	clusterimagepolicySTUFCrPath = "custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"
	stufTestNS                   = "pco-e2e-stuf"
	stufTestImageEnv             = "STUF_TEST_IMAGE"
	stufTrustRootRef             = "serialized-tuf-install-trust-root"
	stufCIPName                  = "serialized-tuf-install-cluster-image-policy"
)

var (
	policyControllerSTUFCrABSPath   string
	trustRootSTUFCrABSPath          string
	clusterImagePolicySTUFCrABSPath string
	stufTestImage                   string
	stufRenderedPolicyontroller     []byte
	stufRenderedTrustRoot           []byte
	stufRenderedClusteImagePolicy   []byte
)

var _ = Describe("policy-controller-operator serializd tuf root", Ordered, Serial, func() {
	var err error

	BeforeAll(func(ctx SpecContext) {
		policyControllerSTUFCrABSPath, err = filepath.Abs(policyControllerSTUFCrPath)
		Expect(err).ToNot(HaveOccurred())

		trustRootSTUFCrABSPath, err = filepath.Abs(trustRootSTUFCrPath)
		Expect(err).ToNot(HaveOccurred())

		clusterImagePolicySTUFCrABSPath, err = filepath.Abs(clusterimagepolicySTUFCrPath)
		Expect(err).ToNot(HaveOccurred())

		By("ensuring the policy-controller-operator namespace exists")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: e2e_utils.InstallNamespace},
		})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))

		By("applying the operator bundle: " + policyControllerSTUFCrPath)
		stufRenderedPolicyontroller, err = e2e_utils.RenderTemplate(policyControllerSTUFCrPath, map[string]string{
			"NS": e2e_utils.InstallNamespace,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, stufRenderedPolicyontroller, "")).To(Succeed())

		stufTestImage = e2e_utils.PrepareImage(ctx, stufTestImageEnv)

		DeferCleanup(func(ctx SpecContext) {
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, "test-pod", stufTestNS)).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, stufTestNS, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, stufCIPName, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, stufTrustRootRef, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "rhtas.charts.redhat.com", Version: "v1alpha1", Kind: "PolicyController"}, "policycontroller-sample", e2e_utils.InstallNamespace)).To(Succeed())
			Expect(e2e_utils.WaitForPolicyControllerResourcesDeleted(ctx, k8sClient)).To(Succeed())
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

		serializedRepo, err := e2e_utils.TufMirrorFS(ctx)
		Expect(err).NotTo(HaveOccurred())

		stufRenderedTrustRoot, err = e2e_utils.RenderTemplate(trustRootSTUFCrABSPath, map[string]string{
			"TUFRoot":    e2e_utils.Base64EncodeString(tufroot),
			"REPOSITORY": e2e_utils.Base64EncodeString(serializedRepo),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, stufRenderedTrustRoot, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-sigstore-keys"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[stufTrustRootRef]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", stufTrustRootRef)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {

	stufRenderedClusteImagePolicy, err = e2e_utils.RenderTemplate(clusterImagePolicySTUFCrABSPath, map[string]string{
		"FULCIO_URL":          e2e_utils.FulcioUrl(),
		"REKOR_URL":           e2e_utils.RekorUrl(),
		"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
		"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
		"TEST_IMAGE":          stufTestImage,
		"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(stufTestImage),
		"TRUST_ROOT_REF":      stufTrustRootRef,
		"CIP_NAME":            stufCIPName,
	})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, stufRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-image-policies"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[stufCIPName]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", stufCIPName)
	})

	It("should create a test namespace", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestNamespace(ctx, k8sClient, stufTestNS)).NotTo(HaveOccurred())

		Eventually(func(g Gomega, ctx SpecContext) {
			ns := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: stufTestNS}, ns)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ns.Status.Phase).To(Equal(corev1.NamespaceActive))
		}).WithContext(ctx).Should(Succeed())
	})

	It("should reject controller creation in wrong namespace", func(ctx SpecContext) {
		stufRenderedPolicyontroller, err = e2e_utils.RenderTemplate(policyControllerSTUFCrABSPath, map[string]string{
			"NS": stufTestNS,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, stufRenderedPolicyontroller, "")).
			To(MatchError(ContainSubstring(`PolicyController objects may only be created in the "policy-controller-operator" namespace`)))
	})

	It("should reject pod creation in a watched namespace and sign the image", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, stufTestNS, stufTestImage)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.VerifyByCosign(ctx, stufTestImage)
	})

	It("should reject pod creation in a watched namespace and attach a provenance", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, stufTestNS, stufTestImage)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachProvenance(ctx, stufTestImage)
	})

	It("should reject pod creation in a watched namespace and attach an SBOM", func(ctx SpecContext) {
		Expect(e2e_utils.CreateTestPod(ctx, k8sClient, stufTestNS, stufTestImage)).
			To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		e2e_utils.AttachSBOM(ctx, stufTestImage)
	})

	It("should accept the pod", func(ctx SpecContext) {
		Eventually(func(ctx SpecContext) error {
			err := e2e_utils.CreateTestPod(ctx, k8sClient, stufTestNS, stufTestImage)
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}).WithContext(ctx).WithPolling(5*time.Second).Should(Succeed(), "pod admission never became allowed")
	})
})
