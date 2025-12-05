package e2e_utils

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx SpecContext, k8sClient client.Client, namespace, testImage string) {
	By("creating a test namespace")
	Expect(CreateTestNamespace(ctx, k8sClient, namespace)).NotTo(HaveOccurred())

	Eventually(func(g Gomega, ctx SpecContext) {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ns.Status.Phase).To(Equal(corev1.NamespaceActive))
	}).WithContext(ctx).Should(Succeed())

	By("rejecting pod creation in a watched namespace and signing the image")
	Expect(CreateTestPod(ctx, k8sClient, namespace, testImage)).
		To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
	VerifyByCosign(ctx, testImage)

	By("rejecting pod creation in a watched namespace and attaching a provenance")
	Expect(CreateTestPod(ctx, k8sClient, namespace, testImage)).
		To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
	AttachProvenance(ctx, testImage)

	By("rejecting pod creation in a watched namespace and attaching an SBOM")
	Expect(CreateTestPod(ctx, k8sClient, namespace, testImage)).
		To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
	AttachSBOM(ctx, testImage)

	By("eventually accepting the pod")
	Eventually(func(ctx SpecContext) error {
		err := CreateTestPod(ctx, k8sClient, namespace, testImage)
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}).WithContext(ctx).WithPolling(5*time.Second).Should(Succeed(), "pod admission never became allowed")

	By("cleaning up test resources")
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, "test-pod", namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, namespace, "")).To(Succeed())
}

func ExpectRequiredResources(ctx SpecContext, k8sClient client.Client) {
	type resource struct {
		name string
		obj  client.Object
	}

	resources := []resource{
		{DeploymentName, &appsv1.Deployment{}},
		{ValidatingWebhookName, &admissionregistrationv1.ValidatingWebhookConfiguration{}},
		{MutatingWebhookName, &admissionregistrationv1.MutatingWebhookConfiguration{}},
		{CipValidatingWebhookName, &admissionregistrationv1.ValidatingWebhookConfiguration{}},
		{CipMutatingWebhookName, &admissionregistrationv1.MutatingWebhookConfiguration{}},
		{WebhookSvc, &corev1.Service{}},
		{MetricsSvc, &corev1.Service{}},
		{SecretName, &corev1.Secret{}},
		{"config-policy-controller", &corev1.ConfigMap{}},
		{"config-image-policies", &corev1.ConfigMap{}},
		{"config-sigstore-keys", &corev1.ConfigMap{}},
		{"policycontroller-sample-policy-controller-webhook-logging", &corev1.ConfigMap{}},
	}

	for _, res := range resources {
		By("checking " + res.name)
		ExpectExists(res.name, InstallNamespace, res.obj, k8sClient, ctx)
	}
}
