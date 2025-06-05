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
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTestPod(ctx context.Context, k8sClient client.Client, ns string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-image",
					Image: TestImage(),
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
	time.Sleep(20) //Allow time for the policy-controller to reconcile
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

func DeleteManifest(ctx context.Context, k8sClient client.Client, data []byte, filepath string) error {
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
	return k8sClient.Delete(ctx, obj)
}

func DeleteResource(ctx context.Context, k8sClient client.Client, gvk schema.GroupVersionKind, name, namespace string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)

	if err := k8sClient.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s %q in namespace %q: %w",
			gvk.Kind, name, namespace, err)
	}
	return nil
}
