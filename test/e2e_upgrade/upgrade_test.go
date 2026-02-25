//go:build upgrade

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e_utils "github.com/securesign/policy-controller-operator/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	operatorInstallNS = "policy-controller-operator"
	operatorSubName   = "policy-controller-operator"

	upgradeTestNS        = "pco-upgrade-e2e"
	upgradeTestImageEnv  = "UPGRADE_TEST_IMAGE"
	postUpgradeTestENV   = "POST_UPGRADE_TEST_IMAGE"
	upgradetrustRootName = "upgrade-install-trust-root"
	upgradeCIPName       = "upgrade-install-cluster-image-policy"

	upgradeOperatorGroupName = "pco-upgrade-operator-group"

	catalogSourceNamespace       = "openshift-marketplace"
	upgradeFromCatalogSourceName = "pco-upgrade-catalog-from"
	upgradeToCatalogSourceName   = "pco-upgrade-catalog-to"
)

var (
	upgradeTestImage     string
	postUpgradeTestImage string
	injectCA             bool

	csvBefore string
	csvAfter  string

	operatorDeploymentsBefore []string
	operatorDeploymentsAfter  []string
)

var _ = Describe("Operator upgrade", Ordered, func() {
	AfterAll(func(ctx SpecContext) {
		installedCSV, err := e2e_utils.GetCSVName(ctx, k8sClient, operatorInstallNS, operatorSubName)
		Expect(err).NotTo(HaveOccurred())
		Expect(installedCSV).NotTo(BeEmpty())

		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, upgradeCIPName, "")).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, upgradetrustRootName, "")).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "rhtas.charts.redhat.com", Version: "v1alpha1", Kind: "PolicyController"}, "policycontroller-sample", e2e_utils.InstallNamespace)).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource"}, upgradeToCatalogSourceName, catalogSourceNamespace)).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource"}, upgradeFromCatalogSourceName, catalogSourceNamespace)).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, operatorSubName, operatorInstallNS)).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "ClusterServiceVersion"}, installedCSV, operatorInstallNS)).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1", Kind: "OperatorGroup"}, upgradeOperatorGroupName, operatorInstallNS)).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, e2e_utils.InstallNamespace, ""))
	})

	BeforeAll(func(ctx SpecContext) {
		upgradeTestImage = e2e_utils.PrepareImage(ctx, upgradeTestImageEnv)
		postUpgradeTestImage = e2e_utils.PrepareImage(ctx, postUpgradeTestENV)

		parsedInjectCA, err := strconv.ParseBool(strings.TrimSpace(e2e_utils.InjectCA()))
		Expect(err).NotTo(HaveOccurred())
		injectCA = parsedInjectCA

		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: e2e_utils.InstallNamespace},
		})).To(SatisfyAny(Succeed(), MatchError(ContainSubstring("already exists"))))
	})

	It("creates an OperatorGroup and a CatalogSource", func(ctx SpecContext) {
		By("ensuring an OperatorGroup exists in " + operatorInstallNS)
		renderedOperatorGroup, err := e2e_utils.RenderTemplate(e2e_utils.OperatorGroupPath, map[string]string{
			"NAME": upgradeOperatorGroupName,
			"NS":   operatorInstallNS,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedOperatorGroup, "")).To(Succeed())

		By("creating a CatalogSource in " + catalogSourceNamespace)
		renderedCatalogSource, err := e2e_utils.RenderTemplate(e2e_utils.CatalogSourcePath, map[string]string{
			"NAME":        upgradeFromCatalogSourceName,
			"NS":          catalogSourceNamespace,
			"INDEX_IMAGE": e2e_utils.EnvOrDefault(fromOperatorIndexImageEnv, defaultOperatorIndexImage),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedCatalogSource, "")).To(Succeed())

		By("waiting for the CatalogSource to become READY")
		Eventually(func(ctx context.Context) (string, error) {
			catalogSource, err := e2e_utils.GetCatalogSource(ctx, k8sClient, catalogSourceNamespace, upgradeFromCatalogSourceName)
			if err != nil {
				return "", err
			}
			return e2e_utils.GetCatalogSourceLastObservedState(catalogSource)
		}).WithContext(ctx).Should(Equal("READY"), "timed out waiting for CatalogSource %s/%s to be READY", catalogSourceNamespace, upgradeFromCatalogSourceName)
	})

	It("creates a Subscription for PCO", func(ctx SpecContext) {
		renderedSubscription, err := e2e_utils.RenderTemplate(e2e_utils.SubscriptionPath, map[string]string{
			"NAME":             operatorSubName,
			"NS":               operatorInstallNS,
			"CHANNEL":          e2e_utils.EnvOrDefault(fromChannelVar, channelDefault),
			"SOURCE":           upgradeFromCatalogSourceName,
			"SOURCE_NAMESPACE": catalogSourceNamespace,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedSubscription, "")).To(Succeed())

		By("waiting for Subscription to report status.installedCSV")
		Eventually(func(ctx context.Context) (string, error) {
			val, err := e2e_utils.GetCSVName(ctx, k8sClient, operatorInstallNS, operatorSubName)
			if err != nil {
				return "", err
			}
			csvBefore = val
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for Subscription %s/%s to have status.installedCSV", operatorInstallNS, operatorSubName)
		Expect(csvBefore).NotTo(BeEmpty())

		By("waiting for installed CSV to be Succeeded")
		Eventually(func(ctx context.Context) (string, error) {
			csv, err := e2e_utils.GetCSV(ctx, k8sClient, operatorInstallNS, csvBefore)
			if err != nil {
				return "", err
			}
			return e2e_utils.GetCSVPhase(csv)
		}).WithContext(ctx).Should(Equal("Succeeded"), "timed out waiting for CSV %s/%s to be Succeeded", operatorInstallNS, csvBefore)

		csv, err := e2e_utils.GetCSV(ctx, k8sClient, operatorInstallNS, csvBefore)
		Expect(err).NotTo(HaveOccurred())
		operatorDeploymentsBefore, err = e2e_utils.GetCSVDeploymentNames(csv)
		Expect(err).NotTo(HaveOccurred())
		Expect(operatorDeploymentsBefore).NotTo(BeEmpty())

		By("waiting for operator deployment to be Ready: " + strings.Join(operatorDeploymentsBefore, ", "))
		for _, deployName := range operatorDeploymentsBefore {
			Eventually(func(ctx context.Context) error {
				return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, operatorInstallNS, deployName)
			}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready", deployName)
		}
	})

	It("creates the policy controller resource", func(ctx SpecContext) {
		By("applying the operator bundle: " + e2e_utils.PolicyControllerCRPath)
		renderedPolicyController, err := e2e_utils.RenderTemplate(e2e_utils.PolicyControllerCRPath, map[string]string{
			"NS": e2e_utils.InstallNamespace,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedPolicyController, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready", e2e_utils.DeploymentName)
	})

	It("injects CA into the policy-controller deployment", func(ctx SpecContext) {
		if !injectCA {
			return
		}

		By("injecting CA")
		Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, e2e_utils.DeploymentName, e2e_utils.InstallNamespace)).To(Succeed())
		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "trusted-ca-bundle", "ca-bundle.crt")
		}).WithContext(ctx).Should(Succeed(), "trusted-ca-bundle never got its ca-bundle.crt")

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready after CA injection", e2e_utils.DeploymentName)
	})

	It("creates a TrustRoot and adds it to the sigstore-keys ConfigMap", func(ctx SpecContext) {
		tufroot, err := e2e_utils.ResolveTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		upgradeRenderedTrustRoot, err := e2e_utils.RenderTemplate(e2e_utils.TrustRootCommonCrPath, map[string]string{
			"TRUST_ROOT_NAME": upgradetrustRootName,
			"TUFMirror":       e2e_utils.TufUrl(),
			"TUFRoot":         e2e_utils.Base64EncodeString(tufroot),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, upgradeRenderedTrustRoot, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-sigstore-keys", upgradetrustRootName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", upgradetrustRootName)
	})

	It("creates a Cluster image policy and adds it to the config-image-policies ConfigMap", func(ctx SpecContext) {
		upgradeRenderedClusterImagePolicy, err := e2e_utils.RenderTemplate(e2e_utils.ClusterimagepolicyCommonCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          upgradeTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(upgradeTestImage),
			"TRUST_ROOT_REF":      upgradetrustRootName,
			"CIP_NAME":            upgradeCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, upgradeRenderedClusterImagePolicy, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-image-policies", upgradeCIPName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", upgradeCIPName)
	})

	It("verifies policy controller behavour", func(ctx SpecContext) {
		e2e_utils.Verify(ctx, k8sClient, upgradeTestNS, upgradeTestImage, true)
	})

	It("upgrades the PCO", func(ctx SpecContext) {
		By("creating a CatalogSource")
		renderedCatalogSource, err := e2e_utils.RenderTemplate(e2e_utils.CatalogSourcePath, map[string]string{
			"NAME":        upgradeToCatalogSourceName,
			"NS":          catalogSourceNamespace,
			"INDEX_IMAGE": e2e_utils.EnvOrDefault(toOperatorIndexImageEnv, ""),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedCatalogSource, "")).To(Succeed())

		Eventually(func(ctx context.Context) (string, error) {
			catalogSource, err := e2e_utils.GetCatalogSource(ctx, k8sClient, catalogSourceNamespace, upgradeToCatalogSourceName)
			if err != nil {
				return "", err
			}
			return e2e_utils.GetCatalogSourceLastObservedState(catalogSource)
		}).WithContext(ctx).Should(Equal("READY"), "timed out waiting for CatalogSource %s/%s to be READY", catalogSourceNamespace, upgradeToCatalogSourceName)

		By("updating the Subscription")
		Eventually(func(ctx context.Context) error {
			return e2e_utils.UpdateSubscriptionSourceAndChannel(ctx, k8sClient, operatorInstallNS, operatorSubName,
				upgradeToCatalogSourceName, e2e_utils.EnvOrDefault(toChannelVar, channelDefault))
		}).WithContext(ctx).Should(Succeed(), "timed out updating Subscription %s/%s", operatorInstallNS, operatorSubName)

		By("waiting for Subscription to report a new Succeeded installedCSV")
		Eventually(func(ctx context.Context) error {
			val, err := e2e_utils.GetCSVName(ctx, k8sClient, operatorInstallNS, operatorSubName)
			if err != nil {
				return err
			}
			if val == csvBefore {
				return fmt.Errorf("installedCSV has not changed yet (still %q)", csvBefore)
			}

			csv, err := e2e_utils.GetCSV(ctx, k8sClient, operatorInstallNS, val)
			if err != nil {
				return err
			}
			phase, err := e2e_utils.GetCSVPhase(csv)
			if err != nil {
				return err
			}
			if phase != "Succeeded" {
				return fmt.Errorf("CSV %q phase is %q", val, phase)
			}

			csvAfter = val
			operatorDeploymentsAfter, err = e2e_utils.GetCSVDeploymentNames(csv)
			if err != nil {
				return err
			}
			if len(operatorDeploymentsAfter) == 0 {
				return fmt.Errorf("no deployments found in CSV %q", val)
			}
			return nil
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for upgraded CSV to differ from %q and be Succeeded", csvBefore)
		Expect(csvAfter).NotTo(Equal(csvBefore))

		By("waiting for operator deployment to be Ready after upgrade: " + strings.Join(operatorDeploymentsAfter, ", "))
		for _, deployName := range operatorDeploymentsAfter {
			Eventually(func(ctx context.Context) error {
				return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, operatorInstallNS, deployName)
			}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready after upgrade", deployName)
		}
	})

	It("verifies policy controller behavour after upgrade", func(ctx SpecContext) {
		if injectCA {
			By("re-injecting CA after upgrade")
			Expect(e2e_utils.InjectCAIntoDeployment(ctx, k8sClient, e2e_utils.DeploymentName, e2e_utils.InstallNamespace)).To(Succeed())
			Eventually(func(ctx context.Context) error {
				return e2e_utils.WaitForDeploymentReady(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
			}).WithContext(ctx).Should(Succeed(), "timed out waiting for Deployment %q to be ready after CA injection", e2e_utils.DeploymentName)
		}

		By("allowing already signed images through")
		e2e_utils.Verify(ctx, k8sClient, upgradeTestNS, upgradeTestImage, false)

		By("testing a new image")
		upgradeRenderedClusterImagePolicy, err := e2e_utils.RenderTemplate(e2e_utils.ClusterimagepolicyCommonCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          postUpgradeTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(postUpgradeTestImage),
			"TRUST_ROOT_REF":      upgradetrustRootName,
			"CIP_NAME":            upgradeCIPName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, upgradeRenderedClusterImagePolicy, "")).To(Succeed())

		Eventually(func(ctx context.Context) error {
			return e2e_utils.WaitForConfigMapKey(ctx, k8sClient, e2e_utils.InstallNamespace, "config-image-policies", upgradeCIPName)
		}).WithContext(ctx).Should(Succeed(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", upgradeCIPName)

		By("rejecting unsigned images")
		e2e_utils.Verify(ctx, k8sClient, upgradeTestNS, postUpgradeTestImage, true)
	})
})
