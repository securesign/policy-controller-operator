package e2e_utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	const (
		deploymentName  = "test-deployment"
		podName         = "test-pod"
		replicaSetName  = "test-replicaset"
		statefulSetName = "test-statefulset"
		daemonSetName   = "test-daemonset"
		jobName         = "test-job"
		cronJobName     = "test-cronjob"
		headlessSvcName = statefulSetName + "-svc"
	)

	workloads := []struct {
		name   string
		create func(SpecContext) error
	}{
		{
			name: podName,
			create: func(ctx SpecContext) error {
				return CreateTestPod(ctx, k8sClient, namespace, testImage, podName)
			},
		},
		{
			name: deploymentName,
			create: func(ctx SpecContext) error {
				return CreateTestDeployment(ctx, k8sClient, namespace, testImage, deploymentName)
			},
		},
		{
			name: replicaSetName,
			create: func(ctx SpecContext) error {
				return CreateTestReplicaSet(ctx, k8sClient, namespace, testImage, replicaSetName)
			},
		},
		{
			name: statefulSetName,
			create: func(ctx SpecContext) error {
				return CreateTestStatefulSet(ctx, k8sClient, namespace, testImage, statefulSetName)
			},
		},
		{
			name: daemonSetName,
			create: func(ctx SpecContext) error {
				return CreateTestDaemonSet(ctx, k8sClient, namespace, testImage, daemonSetName)
			},
		},
		{
			name: jobName,
			create: func(ctx SpecContext) error {
				return CreateTestJob(ctx, k8sClient, namespace, testImage, jobName)
			},
		},
		{
			name: cronJobName,
			create: func(ctx SpecContext) error {
				return CreateTestCronJob(ctx, k8sClient, namespace, testImage, cronJobName)
			},
		},
	}

	assertReject := func(msg string) {
		for _, workload := range workloads {
			By(msg)
			Expect(workload.create(ctx)).
				To(MatchError(ContainSubstring(`admission webhook "policy.rhtas.com" denied the request`)))
		}
	}

	assertReject("rejecting workload creation with unsigned image")
	VerifyByCosign(ctx, testImage)
	assertReject("still rejecting workload creation when only image is signed")
	AttachProvenance(ctx, testImage)
	assertReject("still rejecting workload creation when only image is signed")
	AttachSBOM(ctx, testImage)

	for _, workload := range workloads {
		By("allowing workload creation when image, provenance, and SBOM are all present")
		Expect(workload.create(ctx)).To(Succeed())
	}

	By("cleaning up test resources")
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, podName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, deploymentName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, replicaSetName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, statefulSetName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, daemonSetName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, jobName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, cronJobName, namespace)).To(Succeed())
	Expect(DeleteResource(ctx, k8sClient, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, headlessSvcName, namespace)).To(Succeed())
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
