//go:build integration

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e_utils "github.com/securesign/policy-controller-operator/test/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	commonTestNS        = "pco-e2e"
	commonTestImageEnv  = "COMMON_TEST_IMAGE"
	commonTrustRootName = "common-install-trust-root"
	commonCIPName       = "common-install-cluster-image-policy"

	byokTestNS        = "pco-e2e-byok"
	byokTestImageEnv  = "BYOK_TEST_IMAGE"
	byokTrustRootName = "byok-install-trust-root"
	byokCIPName       = "byok-install-cluster-image-policy"

	stufTestNS        = "pco-e2e-stuf"
	stufTestImageEnv  = "STUF_TEST_IMAGE"
	stufTrustRootName = "serialized-tuf-install-trust-root"
	stufCIPName       = "serialized-tuf-install-cluster-image-policy"
)

var (
	commonTestImage string
	injectCA        bool
	byokImage       string
	stufTestImage   string
)

var _ = Describe("policy-controller-operator common installation", Ordered, Serial, func() {

	BeforeAll(func(ctx SpecContext) {
		commonTestImage = e2e_utils.PrepareImage(ctx, commonTestImageEnv)
		byokImage = e2e_utils.PrepareImage(ctx, byokTestImageEnv)
		stufTestImage = e2e_utils.PrepareImage(ctx, stufTestImageEnv)
	})

	AfterAll(func(ctx SpecContext) {
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, commonCIPName, "")).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, commonTrustRootName, "")).To(Succeed())

		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, byokCIPName, "")).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, byokTrustRootName, "")).To(Succeed())

		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, stufCIPName, "")).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, stufTrustRootName, "")).To(Succeed())
	})

	It("ensuring deployment is ready", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready", e2e_utils.DeploymentName)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		tufroot, err := e2e_utils.ResolveTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		commonRenderedTrustRoot, err := e2e_utils.RenderTemplate(e2e_utils.TrustRootCommonCrPath, map[string]string{
			"TRUST_ROOT_NAME": commonTrustRootName,
			"TUFMirror":       e2e_utils.TufUrl(),
			"TUFRoot":         e2e_utils.Base64EncodeString(tufroot),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedTrustRoot, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-sigstore-keys", commonTrustRootName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", commonTrustRootName)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {
		commonRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(e2e_utils.ClusterimagepolicyCommonCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          commonTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(commonTestImage),
			"TRUST_ROOT_REF":      commonTrustRootName,
			"CIP_NAME":            commonCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-image-policies", commonCIPName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", commonCIPName)
	})

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, commonTestNS, commonTestImage, true)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		trustedrootValues, err := e2e_utils.ParseTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())
		byokRenderedTrustRoot, err := e2e_utils.RenderTemplate(e2e_utils.TrustRootBYOKCrPath, map[string]string{
			"TRUST_ROOT_NAME":      byokTrustRootName,
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

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-sigstore-keys", byokTrustRootName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", byokTrustRootName)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {
		byokRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(e2e_utils.ClusterimagepolicyBYOKCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          byokImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(byokImage),
			"TRUST_ROOT_REF":      byokTrustRootName,
			"CIP_NAME":            byokCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, byokRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-image-policies", byokCIPName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", byokCIPName)
	})

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, byokTestNS, byokImage, true)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		tufroot, err := e2e_utils.ResolveTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		serializedRepo, err := e2e_utils.TufMirrorFS(ctx)
		Expect(err).NotTo(HaveOccurred())

		stufRenderedTrustRoot, err := e2e_utils.RenderTemplate(e2e_utils.TrustRootSTUFCrPath, map[string]string{
			"TRUST_ROOT_NAME": stufTrustRootName,
			"TUFRoot":         e2e_utils.Base64EncodeString(tufroot),
			"REPOSITORY":      e2e_utils.Base64EncodeString(serializedRepo),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, stufRenderedTrustRoot, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-sigstore-keys", stufTrustRootName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", stufTrustRootName)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {

		stufRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(e2e_utils.ClusterimagepolicySTUFCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          stufTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(stufTestImage),
			"TRUST_ROOT_REF":      stufTrustRootName,
			"CIP_NAME":            stufCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, stufRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-image-policies", stufCIPName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", stufCIPName)
	})

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, stufTestNS, stufTestImage, true)
	})
})
