package e2e_utils

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	defaultInjectCA = "false"
	injectCA        = "INJECT_CA"
)

var (
	GVKCatalogSource = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource"}
	GVKSubscription  = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}
	GVKCSV           = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "ClusterServiceVersion"}
)

func InjectCA() string {
	return EnvOrDefault(injectCA, defaultInjectCA)
}

func CreateTestPod(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-image",
					Image: testImage,
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: "RuntimeDefault",
						},
					},
				},
			},
		},
	}
	err := k8sClient.Create(ctx, pod)
	return err
}

func CreateTestNamespace(ctx context.Context, k8sClient client.Client, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"policy.rhtas.com/include": "true",
			},
		},
	}
	err := k8sClient.Create(ctx, ns)
	time.Sleep(20 * time.Second) //Allow time for the policy-controller to reconcile
	return err
}

func ApplyManifest(ctx context.Context, k8sClient client.Client, data []byte, filepath string) error {
	var err error

	if data == nil {
		data, err = os.ReadFile(filepath)
		if err != nil {
			return err
		}
	}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err = dec.Decode(data, nil, obj)
	if err != nil {
		return fmt.Errorf("error decoding %s into Unstructured: %w", filepath, err)
	}
	return k8sClient.Patch(ctx, obj, client.Apply, client.FieldOwner("policy-controller-operator"))
}

func DeleteResource(ctx context.Context, k8sClient client.Client, gvk schema.GroupVersionKind, name, namespace string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	if namespace != "" {
		obj.SetNamespace(namespace)
	}

	if err := k8sClient.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s %q: %w", gvk.Kind, name, err)
	}

	key := client.ObjectKey{Name: name, Namespace: namespace}
	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.5,
		Steps:    10,
		Jitter:   0.1,
	}
	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		getErr := k8sClient.Get(ctx, key, obj)
		switch {
		case errors.IsNotFound(getErr):
			return true, nil
		case getErr != nil:
			return false, getErr
		default:
			return false, nil
		}
	})
}

func InjectCAIntoDeployment(ctx context.Context, k8sClient client.Client, deploymentName, namespace string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trusted-ca-bundle",
			Namespace: namespace,
			Labels: map[string]string{
				"config.openshift.io/inject-trusted-cabundle": "true",
			},
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, k8sClient, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("ensuring trusted CA ConfigMap: %w", err)
	}

	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deploymentName}, &deploy); err != nil {
		return err
	}

	ensureVolume(&deploy, corev1.Volume{
		Name: "trusted-ca",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "trusted-ca-bundle"},
				Items: []corev1.KeyToPath{
					{Key: "ca-bundle.crt", Path: "ca-bundle.crt"},
				},
			},
		},
	})

	for i := range deploy.Spec.Template.Spec.Containers {
		ensureMount(&deploy.Spec.Template.Spec.Containers[i], corev1.VolumeMount{
			Name:      "trusted-ca",
			MountPath: "/etc/ssl/certs/tls-ca-bundle.pem",
			SubPath:   "ca-bundle.crt",
			ReadOnly:  true,
		})
		ensureEnv(&deploy.Spec.Template.Spec.Containers[i], corev1.EnvVar{
			Name:  "SSL_CERT_DIR",
			Value: "/var/run/secrets/kubernetes.io/serviceaccount:/etc/ssl/certs",
		})
	}
	return k8sClient.Update(ctx, &deploy)
}

func ensureVolume(d *appsv1.Deployment, v corev1.Volume) {
	spec := &d.Spec.Template.Spec
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == v.Name {
			spec.Volumes[i] = v
			return
		}
	}
	spec.Volumes = append(spec.Volumes, v)
}

func ensureMount(c *corev1.Container, m corev1.VolumeMount) {
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == m.Name {
			c.VolumeMounts[i] = m
			return
		}
	}
	c.VolumeMounts = append(c.VolumeMounts, m)
}

func ensureEnv(c *corev1.Container, e corev1.EnvVar) {
	for i := range c.Env {
		if c.Env[i].Name == e.Name {
			c.Env[i] = e
			return
		}
	}
	c.Env = append(c.Env, e)
}

func CreateTestDeployment(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	labels := map[string]string{
		"app": name,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-image",
							Image: testImage,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr.To(true),
								SeccompProfile: &corev1.SeccompProfile{
									Type: "RuntimeDefault",
								},
							},
						},
					},
				},
			},
		},
	}
	return k8sClient.Create(ctx, deployment)
}

func CreateTestReplicaSet(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	labels := map[string]string{
		"app": name,
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test-image",
							Image: testImage,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr.To(true),
								SeccompProfile: &corev1.SeccompProfile{
									Type: "RuntimeDefault",
								},
							},
						},
					},
				},
			},
		},
	}

	return k8sClient.Create(ctx, rs)
}

func CreateTestStatefulSet(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	labels := map[string]string{
		"app": name,
	}
	serviceName := name + "-svc"

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
		},
	}
	if err := k8sClient.Create(ctx, svc); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating headless service for statefulset: %w", err)
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: serviceName,
			Replicas:    ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test-image",
							Image: testImage,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr.To(true),
								SeccompProfile: &corev1.SeccompProfile{
									Type: "RuntimeDefault",
								},
							},
						},
					},
				},
			},
		},
	}

	return k8sClient.Create(ctx, sts)
}

func CreateTestDaemonSet(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	labels := map[string]string{
		"app": name,
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test-image",
							Image: testImage,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr.To(true),
								SeccompProfile: &corev1.SeccompProfile{
									Type: "RuntimeDefault",
								},
							},
						},
					},
				},
			},
		},
	}

	return k8sClient.Create(ctx, ds)
}

func CreateTestJob(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	labels := map[string]string{
		"app": name,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To[int32](0),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "test-image",
							Image: testImage,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr.To(true),
								SeccompProfile: &corev1.SeccompProfile{
									Type: "RuntimeDefault",
								},
							},
						},
					},
				},
			},
		},
	}
	return k8sClient.Create(ctx, job)
}

func CreateTestCronJob(ctx context.Context, k8sClient client.Client, ns, testImage, name string) error {
	labels := map[string]string{
		"app": name,
	}

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/1 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit: ptr.To[int32](0),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  "test-image",
									Image: testImage,
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{"ALL"},
										},
										RunAsNonRoot: ptr.To(true),
										SeccompProfile: &corev1.SeccompProfile{
											Type: "RuntimeDefault",
										},
									},
								},
							},
						},
					},
				},
			},
			SuccessfulJobsHistoryLimit: ptr.To[int32](1),
			FailedJobsHistoryLimit:     ptr.To[int32](1),
		},
	}
	return k8sClient.Create(ctx, cronJob)
}

func WaitForDeploymentReady(ctx context.Context, c client.Client, ns, name string) error {
	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, dep); err != nil {
		return err
	}
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	if dep.Status.ReadyReplicas != desired {
		return fmt.Errorf("ready %d/%d", dep.Status.ReadyReplicas, desired)
	}
	return nil
}

func WaitForConfigMapKey(ctx context.Context, c client.Client, ns, name, key string) error {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, cm); err != nil {
		return err
	}
	val, ok := cm.Data[key]
	if !ok || val == "" {
		return fmt.Errorf("key not present yet")
	}
	return nil
}

func GetUnstructured(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func GetCatalogSource(ctx context.Context, c client.Client, ns, name string) (*unstructured.Unstructured, error) {
	return GetUnstructured(ctx, c, GVKCatalogSource, ns, name)
}
func GetSubscription(ctx context.Context, c client.Client, ns, name string) (*unstructured.Unstructured, error) {
	return GetUnstructured(ctx, c, GVKSubscription, ns, name)
}
func GetCSV(ctx context.Context, c client.Client, ns, name string) (*unstructured.Unstructured, error) {
	return GetUnstructured(ctx, c, GVKCSV, ns, name)
}

func GetCatalogSourceLastObservedState(cs *unstructured.Unstructured) (string, error) {
	return GetNestedString(cs.Object, "status", "connectionState", "lastObservedState")
}

func GetCSVPhase(csv *unstructured.Unstructured) (string, error) {
	return GetNestedString(csv.Object, "status", "phase")
}

func GetCSVName(ctx context.Context, c client.Client, ns, subName string) (string, error) {
	sub, err := GetUnstructured(ctx, c, GVKSubscription, ns, subName)
	if err != nil {
		return "", err
	}
	val, err := GetNestedString(sub.Object, "status", "installedCSV")
	if err != nil {
		return "", fmt.Errorf("Subscription %s/%s: %w", ns, subName, err)
	}
	return val, nil
}

func GetCSVDeploymentNames(csv *unstructured.Unstructured) ([]string, error) {
	deployments, found, err := unstructured.NestedSlice(csv.Object, "spec", "install", "spec", "deployments")
	if err != nil {
		return nil, err
	}
	if !found || len(deployments) == 0 {
		return nil, fmt.Errorf("spec.install.spec.deployments not present or empty")
	}

	names := make([]string, 0, len(deployments))
	for i, raw := range deployments {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected type for deployments[%d]: %T", i, raw)
		}
		nameAny, ok := m["name"]
		name, _ := nameAny.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("missing deployments[%d].name", i)
		}
		names = append(names, name)
	}
	return names, nil
}

func UpdateSubscriptionSourceAndChannel(ctx context.Context, c client.Client, ns, name, source, channel string) error {
	sub, err := GetSubscription(ctx, c, ns, name)
	if err != nil {
		return err
	}

	orig := sub.DeepCopy()
	if err := unstructured.SetNestedField(sub.Object, source, "spec", "source"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(sub.Object, channel, "spec", "channel"); err != nil {
		return err
	}

	return c.Patch(ctx, sub, client.MergeFrom(orig))
}
