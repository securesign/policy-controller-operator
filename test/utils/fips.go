package e2e_utils

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	restConfig *rest.Config
	clientset  *kubernetes.Clientset
)

// SetRestConfig makes the REST config available to ExecInPod. Suite setup
// must call this alongside constructing the controller-runtime client.
func SetRestConfig(cfg *rest.Config) {
	restConfig = cfg
	cs, err := kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred(), "building clientset for ExecInPod")
	clientset = cs
}

func ExecInPod(podName, containerName, namespace string, command ...string) (string, error) {
	if clientset == nil {
		return "", fmt.Errorf("ExecInPod: rest config not set, call SetRestConfig during suite setup")
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		return "", fmt.Errorf("pod exec failed: %w, stderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func getRunningPods(ctx context.Context, k8sClient client.Client, namespace string, selector map[string]string) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels(selector),
	); err != nil {
		return nil, err
	}
	var running []*corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp == nil {
			running = append(running, pod)
		}
	}
	return running, nil
}

func GetDeploymentPod(ctx context.Context, k8sClient client.Client, namespace, deploymentName string) (*corev1.Pod, string) {
	deploy := &appsv1.Deployment{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deploymentName}, deploy)).To(Succeed())

	pods, err := getRunningPods(ctx, k8sClient, namespace, deploy.Spec.Selector.MatchLabels)
	Expect(err).ToNot(HaveOccurred())
	Expect(pods).ToNot(BeEmpty(), "no running pod found for deployment %s", deploymentName)

	pod := pods[0]
	Expect(pod.Spec.Containers).ToNot(BeEmpty())
	return pod, pod.Spec.Containers[0].Name
}

func FindOperatorDeployment(ctx context.Context, k8sClient client.Client) *appsv1.Deployment {
	depList := &appsv1.DeploymentList{}
	Expect(k8sClient.List(ctx, depList,
		client.MatchingLabels{"control-plane": "policy-controller-operator"},
	)).To(Succeed())
	Expect(depList.Items).ToNot(BeEmpty(),
		"could not find operator deployment with label control-plane=policy-controller-operator")
	return &depList.Items[0]
}

func PatchDeploymentEnv(ctx context.Context, k8sClient client.Client, namespace, deploymentName string, env corev1.EnvVar) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deploy := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deploymentName}, deploy); err != nil {
			return err
		}
		for i := range deploy.Spec.Template.Spec.Containers {
			ensureEnv(&deploy.Spec.Template.Spec.Containers[i], env)
		}
		return k8sClient.Update(ctx, deploy)
	})
	Expect(err).ToNot(HaveOccurred())
}

func hasEnvVars(pod *corev1.Pod, envs []corev1.EnvVar) bool {
	for _, want := range envs {
		found := false
		for _, c := range pod.Spec.Containers {
			for _, got := range c.Env {
				if got.Name == want.Name && got.Value == want.Value {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func WaitForDeploymentPodWithEnv(ctx context.Context, k8sClient client.Client, namespace, deploymentName string, envs ...corev1.EnvVar) *corev1.Pod {
	deploy := &appsv1.Deployment{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deploymentName}, deploy)).To(Succeed())
	selector := deploy.Spec.Selector.MatchLabels

	var result *corev1.Pod
	Eventually(func(g Gomega, ctx context.Context) {
		pods, err := getRunningPods(ctx, k8sClient, namespace, selector)
		g.Expect(err).ToNot(HaveOccurred())

		for _, pod := range pods {
			ready := len(pod.Status.ContainerStatuses) > 0
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			if hasEnvVars(pod, envs) {
				result = pod
				return
			}
		}
		g.Expect(result).ToNot(BeNil(), "waiting for ready pod with required env vars in deployment %s", deploymentName)
	}).WithContext(ctx).Should(Succeed())
	return result
}

func VerifyFIPSKernel(podName, containerName, namespace string) {
	out, err := ExecInPod(podName, containerName, namespace, "cat", "/proc/sys/crypto/fips_enabled")
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(out)).To(Equal("1"), "kernel FIPS mode should be enabled")
}

func VerifyGodebugFIPSOnly(podName, containerName, namespace string) {
	out, err := ExecInPod(podName, containerName, namespace, "printenv", "GODEBUG")
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(out)).To(Equal("fips140=only"),
		fmt.Sprintf("container %s should have GODEBUG=fips140=only", containerName))
}

func VerifyBinaryGOFIPS(podName, containerName, namespace, binaryPath string) {
	out, err := ExecInPod(podName, containerName, namespace,
		"grep", "-aom", "1", "GOFIPS140=", binaryPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(out)).To(ContainSubstring("GOFIPS140="),
		fmt.Sprintf("binary %s should be built with GOFIPS140", binaryPath))
}

func GetBinaryPath(podName, containerName, namespace string) string {
	out, err := ExecInPod(podName, containerName, namespace, "readlink", "-f", "/proc/1/exe")
	Expect(err).ToNot(HaveOccurred())
	path := strings.TrimSpace(out)
	Expect(path).ToNot(BeEmpty(), "could not determine binary path from /proc/1/exe")
	return path
}

func VerifyFIPSGoNative(podName, containerName, namespace string) {
	VerifyFIPSKernel(podName, containerName, namespace)
	VerifyGodebugFIPSOnly(podName, containerName, namespace)
	binaryPath := GetBinaryPath(podName, containerName, namespace)
	VerifyBinaryGOFIPS(podName, containerName, namespace, binaryPath)
}

var (
	fipsEnabled  bool
	fipsResolved bool
)

func checkFIPSKernel(podName, containerName, namespace string) (bool, error) {
	if fipsResolved {
		return fipsEnabled, nil
	}
	out, err := ExecInPod(podName, containerName, namespace, "cat", "/proc/sys/crypto/fips_enabled")
	if err != nil {
		return false, err
	}
	fipsEnabled = strings.TrimSpace(out) == "1"
	fipsResolved = true
	return fipsEnabled, nil
}

// IsFIPSCluster checks whether the host kernel has FIPS enabled.
// The result is cached after the first successful check; transient
// ExecInPod failures are retried via Eventually.
func IsFIPSCluster(podName, containerName, namespace string) bool {
	var isFIPS bool
	Eventually(func() error {
		var err error
		isFIPS, err = checkFIPSKernel(podName, containerName, namespace)
		return err
	}).Should(Succeed(), "failed to detect FIPS status by execing into pod %s/%s", namespace, podName)
	if isFIPS {
		fmt.Println("  FIPS detection: cluster IS FIPS-enabled")
	} else {
		fmt.Println("  FIPS detection: cluster is NOT FIPS-enabled")
	}
	return isFIPS
}

func DetectAndConfigureFIPS(ctx context.Context, k8sClient client.Client) {
	pod, containerName := GetDeploymentPod(ctx, k8sClient, InstallNamespace, DeploymentName)

	if !IsFIPSCluster(pod.Name, containerName, pod.Namespace) {
		return
	}

	godebug := corev1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}

	By("patching operator CSV/deployment with GODEBUG=fips140=only")
	operatorDep := FindOperatorDeployment(ctx, k8sClient)
	patchOperatorFIPSEnv(ctx, k8sClient, operatorDep, godebug)
	WaitForDeploymentPodWithEnv(ctx, k8sClient, operatorDep.Namespace, operatorDep.Name, godebug)

	By("patching webhook deployment with GODEBUG=fips140=only")
	PatchDeploymentEnv(ctx, k8sClient, InstallNamespace, DeploymentName, godebug)
	WaitForDeploymentPodWithEnv(ctx, k8sClient, InstallNamespace, DeploymentName, godebug)
}

func patchOperatorFIPSEnv(ctx context.Context, k8sClient client.Client, deploy *appsv1.Deployment, env corev1.EnvVar) {
	csvName := findCSVForDeployment(ctx, k8sClient, deploy)
	if csvName != "" {
		patchCSVDeploymentEnv(ctx, k8sClient, deploy.Namespace, csvName, deploy.Name, env)
		return
	}
	PatchDeploymentEnv(ctx, k8sClient, deploy.Namespace, deploy.Name, env)
}

func findCSVForDeployment(ctx context.Context, k8sClient client.Client, deploy *appsv1.Deployment) string {
	for _, ref := range deploy.OwnerReferences {
		if ref.Kind == "ClusterServiceVersion" {
			return ref.Name
		}
	}
	csvName, err := GetCSVName(ctx, k8sClient, deploy.Namespace, PackageName)
	if err != nil || csvName == "" {
		return ""
	}
	return csvName
}

func patchCSVDeploymentEnv(ctx context.Context, k8sClient client.Client, namespace, csvName, deployName string, env corev1.EnvVar) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		csv, err := GetCSV(ctx, k8sClient, namespace, csvName)
		if err != nil {
			return err
		}

		deployments, found, err := unstructured.NestedSlice(csv.Object, "spec", "install", "spec", "deployments")
		if err != nil || !found {
			return fmt.Errorf("CSV %s has no spec.install.spec.deployments: %w", csvName, err)
		}

		for i, raw := range deployments {
			dep, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := dep["name"].(string)
			if name != deployName {
				continue
			}

			containers, _, _ := unstructured.NestedSlice(dep, "spec", "template", "spec", "containers")
			for j, cRaw := range containers {
				c, ok := cRaw.(map[string]any)
				if !ok {
					continue
				}
				envList, _, _ := unstructured.NestedSlice(c, "env")
				updated := false
				for k, eRaw := range envList {
					e, ok := eRaw.(map[string]any)
					if !ok {
						continue
					}
					if e["name"] == env.Name {
						envList[k] = map[string]any{"name": env.Name, "value": env.Value}
						updated = true
						break
					}
				}
				if !updated {
					envList = append(envList, map[string]any{"name": env.Name, "value": env.Value})
				}
				c["env"] = envList
				containers[j] = c
			}
			if err := unstructured.SetNestedSlice(dep, containers, "spec", "template", "spec", "containers"); err != nil {
				return err
			}
			deployments[i] = dep
		}

		if err := unstructured.SetNestedSlice(csv.Object, deployments, "spec", "install", "spec", "deployments"); err != nil {
			return err
		}
		return k8sClient.Update(ctx, csv)
	})
	Expect(err).ToNot(HaveOccurred(), "failed to patch CSV %s deployment %s with %s=%s", csvName, deployName, env.Name, env.Value)
}
