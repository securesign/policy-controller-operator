package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e_utils "github.com/securesign/policy-controller-operator/test/e2e/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	updateTrustRootName           = "update-trust-root"
	updatedClusterImagePolicyName = "update-cip"
)

var _ = Describe("policy-controller-operator update reconciliation", Ordered, Serial, func() {

	AfterAll(func(ctx SpecContext) {
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1beta1", Kind: "ClusterImagePolicy"}, updatedClusterImagePolicyName, "")).To(Succeed())
		Expect(e2e_utils.DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "policy.sigstore.dev", Version: "v1alpha1", Kind: "TrustRoot"}, updateTrustRootName, "")).To(Succeed())
	})

	It("ensuring deployment is ready", func(ctx SpecContext) {
		dep := &appsv1.Deployment{}
		e2e_utils.ExpectExists(e2e_utils.DeploymentName, e2e_utils.InstallNamespace, dep, k8sClient, ctx)
		desired := *dep.Spec.Replicas
		Eventually(func(ctx context.Context) int32 {
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: e2e_utils.DeploymentName}, dep)
			Expect(err).ToNot(HaveOccurred())
			return dep.Status.ReadyReplicas
		}).WithContext(ctx).Should(Equal(desired), "timed out waiting for %d pods to be Ready in Deployment %q", desired, e2e_utils.DeploymentName)
	})

	It("reconciles PolicyController", func(ctx SpecContext) {
		logConfigMapName := fmt.Sprintf("%s-logging", e2e_utils.DeploymentName)
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: logConfigMapName}, cm)).To(Succeed())
		originalWebhookLogLevel := cm.Data["loglevel.webhook"]

		pc, err := e2e_utils.GetPolicyController(ctx, k8sClient, e2e_utils.InstallNamespace, "policycontroller-sample")
		Expect(err).NotTo(HaveOccurred())
		Expect(pc).To(Not(BeNil()))

		Expect(unstructured.SetNestedField(pc.Object, "debug", "spec", "policy-controller", "loglevel")).To(Succeed())
		Expect(k8sClient.Update(ctx, pc)).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: logConfigMapName}, cm); err != nil {
				return "", err
			}
			return cm.Data["loglevel.webhook"], nil
		}).WithContext(ctx).Should(Equal("debug"))

		Expect(originalWebhookLogLevel).NotTo(Equal("debug"), "loglevel.webhook was already debug before the update")
	})

	It("reconciles TrustRoot", func(ctx SpecContext) {
		tufroot, err := e2e_utils.ResolveTufRoot(ctx)
		Expect(err).NotTo(HaveOccurred())

		renderedTrustRoot, err := e2e_utils.RenderTemplate(trustRootCommonCrPath, map[string]string{
			"TRUST_ROOT_NAME": updateTrustRootName,
			"TUFMirror":       e2e_utils.TufUrl(),
			"TUFRoot":         e2e_utils.Base64EncodeString(tufroot),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, renderedTrustRoot, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-sigstore-keys"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[updateTrustRootName]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-sigstore-keys' to have the %s key", updateTrustRootName)

		var trustRootGeneration int64
		Eventually(func(ctx SpecContext) (int64, error) {
			trustRoot, err := e2e_utils.GetTrustRoot(ctx, k8sClient, updateTrustRootName)
			if err != nil {
				return 0, err
			}

			generation, found, err := unstructured.NestedInt64(trustRoot.Object, "status", "observedGeneration")
			if err != nil {
				return 0, err
			}
			if !found {
				return 0, fmt.Errorf("observedGeneration not set yet")
			}

			trustRootGeneration = generation
			return generation, nil
		}).WithContext(ctx).Should(BeNumerically(">", int64(0)))

		newTufUrl := "https://tuf-repo-cdn.sigstore.dev"
		Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			trustRoot, err := e2e_utils.GetTrustRoot(ctx, k8sClient, updateTrustRootName)
			if err != nil {
				return err
			}

			if err := unstructured.SetNestedField(trustRoot.Object, newTufUrl, "spec", "remote", "mirror"); err != nil {
				return err
			}

			return k8sClient.Update(ctx, trustRoot)
		})).To(Succeed())

		Eventually(func(ctx SpecContext) (int64, error) {
			tr, err := e2e_utils.GetTrustRoot(ctx, k8sClient, updateTrustRootName)
			if err != nil {
				return 0, err
			}

			foundTrustRootGeneration, found, err := unstructured.NestedInt64(tr.Object, "status", "observedGeneration")
			if err != nil {
				return 0, err
			}
			if !found {
				return 0, fmt.Errorf("observedGeneration not set yet")
			}
			return foundTrustRootGeneration, nil
		}).WithContext(ctx).Should(BeNumerically(">=", trustRootGeneration+1))

	})

	It("reconciles ClusterImagePolicy", func(ctx SpecContext) {
		commonRenderedClusteImagePolicy, err := e2e_utils.RenderTemplate(clusterimagepolicyCommonCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": e2e_utils.OidcIssuerSubject(),
			"TEST_IMAGE":          commonTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(commonTestImage),
			"TRUST_ROOT_REF":      updateTrustRootName,
			"CIP_NAME":            updatedClusterImagePolicyName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, commonRenderedClusteImagePolicy, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: e2e_utils.InstallNamespace, Name: "config-image-policies"}, cm); err != nil {
				return "", err
			}
			val, ok := cm.Data[updatedClusterImagePolicyName]
			if !ok {
				return "", fmt.Errorf("key not present yet")
			}
			return val, nil
		}).WithContext(ctx).ShouldNot(BeEmpty(), "timed out waiting for ConfigMap 'config-image-policies' to have the %s key", updatedClusterImagePolicyName)

		var cipGeneration int64
		Eventually(func(ctx SpecContext) (int64, error) {
			cip, err := e2e_utils.GetClusterImagePolicy(ctx, k8sClient, updatedClusterImagePolicyName)
			if err != nil {
				return 0, err
			}

			generation, found, err := unstructured.NestedInt64(cip.Object, "status", "observedGeneration")
			if err != nil {
				return 0, err
			}
			if !found {
				return 0, fmt.Errorf("observedGeneration not set yet")
			}

			cipGeneration = generation
			return generation, nil
		}).WithContext(ctx).Should(BeNumerically(">", int64(0)))

		updatedSubject := fmt.Sprintf("%s-updated", e2e_utils.OidcIssuerSubject())
		updatedClusterImagePolicy, err := e2e_utils.RenderTemplate(clusterimagepolicyCommonCrPath, map[string]string{
			"FULCIO_URL":          e2e_utils.FulcioUrl(),
			"REKOR_URL":           e2e_utils.RekorUrl(),
			"OIDC_ISSUER_URL":     e2e_utils.OidcIssuerUrl(),
			"OIDC_ISSUER_SUBJECT": updatedSubject,
			"TEST_IMAGE":          commonTestImage,
			"TEST_IMAGE_PREFIX":   e2e_utils.ImageRepoPrefix(commonTestImage),
			"TRUST_ROOT_REF":      updateTrustRootName,
			"CIP_NAME":            updatedClusterImagePolicyName,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(e2e_utils.ApplyManifest(ctx, k8sClient, updatedClusterImagePolicy, "")).To(Succeed())

		Eventually(func(ctx SpecContext) (int64, error) {
			cip, err := e2e_utils.GetClusterImagePolicy(ctx, k8sClient, updatedClusterImagePolicyName)
			if err != nil {
				return 0, err
			}
			Expect(cip).To(Not(BeNil()))

			foundCipGeneration, found, err := unstructured.NestedInt64(cip.Object, "status", "observedGeneration")
			if err != nil {
				return 0, err
			}
			if !found {
				return 0, fmt.Errorf("observedGeneration not set yet")
			}

			return foundCipGeneration, nil
		}).WithContext(ctx).Should(BeNumerically(">=", cipGeneration+1))

	})
})
