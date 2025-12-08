package e2e

import (
	"fmt"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e_utils "github.com/securesign/policy-controller-operator/test/e2e/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	policyControllerCRPath         = "custom_resources/policy_controller/common_policy_controller.yaml.tpl"
	trustRootCommonCrPath          = "custom_resources/trust_roots/common_trust_root.yaml.tpl"
	clusterimagepolicyCommonCrPath = "custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"
	commonTestNS                   = "pco-e2e"
	commonTestImageEnv             = "COMMON_TEST_IMAGE"
	commonTrustRootRef             = "common-install-trust-root"
	commonCIPName                  = "common-install-cluster-image-policy"

	trustRootBYOKCrPath          = "custom_resources/trust_roots/byok_trust_root.yaml.tpl"
	clusterimagepolicyBYOKCrPath = "custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"
	byokTestNS                   = "pco-e2e-byok"
	byokTestImageEnv             = "BYOK_TEST_IMAGE"
	byokTrustRootRef             = "byok-install-trust-root"
	byokCIPName                  = "byok-install-cluster-image-policy"

	trustRootSTUFCrPath          = "custom_resources/trust_roots/stuf_trust_root.yaml.tpl"
	clusterimagepolicySTUFCrPath = "custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"
	stufTestNS                   = "pco-e2e-stuf"
	stufTestImageEnv             = "STUF_TEST_IMAGE"
	stufTrustRootRef             = "serialized-tuf-install-trust-root"
	stufCIPName                  = "serialized-tuf-install-cluster-image-policy"
)

var (
	commonTestImage string
	injectCA        bool
	byokImage       string
	stufTestImage   string
)

var _ = Describe("policy-controller-operator common installation", Ordered, Serial, func() {

	BeforeAll(func(ctx SpecContext) {
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

		commonTestImage = e2e_utils.PrepareImage(ctx, commonTestImageEnv)
		byokImage = e2e_utils.PrepareImage(ctx, byokTestImageEnv)
		stufTestImage = e2e_utils.PrepareImage(ctx, stufTestImageEnv)
		injectCA, err = strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func(ctx SpecContext) {
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, commonCIPName, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, commonTrustRootRef, "")).To(Succeed())

			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, byokCIPName, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, byokTrustRootRef, "")).To(Succeed())

			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, stufCIPName, "")).To(Succeed())
			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, stufTrustRootRef, "")).To(Succeed())

			Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "rhtas.charts.redhat.com", Version: "v1alpha1", Kind: "PolicyController"}, "policycontroller-sample", e2e_utils.InstallNamespace)).To(Succeed())
		})
	})

	It("creates all required resources", func(ctx SpecContext) {
		e2e_utils.ExpectRequiredResources(ctx, k8sClient)
	})

	It("should reject controller creation in wrong namespace", func(ctx SpecContext) {
		renderedPolicyController, err := e2e_utils.RenderTemplate(policyControllerCRPath, map[string]string{
			"NS": "default",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedPolicyController, "")).
			To(MatchError(ContainSubstring(`PolicyController objects may only be created in the "policy-controller-operator" namespace`)))
	})

	It("eventually has the deployment ready", func(ctx SpecContext) {
		dep := &appsv1.Deployment{}
		e2e_utils.ExpectExists(e2e_utils.DeploymentName, e2e_utils.InstallNamespace, dep, k8sClient, ctx)

		desired := *dep.Spec.Replicas
		Eventually(func(ctx SpecContext) int32 {
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)
			Expect(err).ToNot(HaveOccurred())
			return dep.Status.ReadyReplicas
		}).WithContext(ctx).Should(Equal(desired), "timed out waiting for %d pods to be Ready in Deployment %q", desired, e2e_utils.DeploymentName)
	})

	It("injects the CA bundle and the Deployment rolls out", func(ctx SpecContext) {
		if !injectCA {
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

		commonRenderedTrustRoot, err := e2e_utils.RenderTemplate(trustRootCommonCrPath, map[string]string{
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
		commonRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(clusterimagepolicyCommonCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          commonTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(commonTestImage),
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

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, commonTestNS, commonTestImage)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		trustedrootValues, err := e2e_utils.ParseTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		byokRenderedTrustRoot, err := e2e_utils.RenderTemplate(trustRootBYOKCrPath, map[string]string{
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

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-sigstore-keys"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[byokTrustRootRef]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", byokTrustRootRef)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {
		byokRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(clusterimagepolicyBYOKCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          byokImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(byokImage),
			"TRUST_ROOT_REF":      byokTrustRootRef,
			"CIP_NAME":            byokCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, byokRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-image-policies"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[byokCIPName]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", byokCIPName)
	})

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, byokTestNS, byokImage)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		tufroot, err := e2e_utils.ResolveTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		serializedRepo, err := e2e_utils.TufMirrorFS(ctx)
		Expect(err).NotTo(HaveOccurred())

		stufRenderedTrustRoot, err := e2e_utils.RenderTemplate(trustRootSTUFCrPath, map[string]string{
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

		stufRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(clusterimagepolicySTUFCrPath, map[string]string{
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

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, stufTestNS, stufTestImage)
	})
})
