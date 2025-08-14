package e2e_utils

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultInjectCA = "false"
	injectCA        = "INJECT_CA"
)

func InjectCA() string {
	return EnvOrDefault(injectCA, defaultInjectCA)
}

func CreateTestPod(ctx context.Context, k8sClient client.Client, ns, testImage string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-image",
					Image: testImage,
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
	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deploymentName}, &deploy); err != nil {
		return err
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		ensureEnv(&deploy.Spec.Template.Spec.Containers[i], corev1.EnvVar{
			Name:  "SSL_CERT_DIR",
			Value: "/var/run/secrets/kubernetes.io/serviceaccount:/etc/custom-ca:/etc/ssl/certs",
		})
	}
	return k8sClient.Update(ctx, &deploy)
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
