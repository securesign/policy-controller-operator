package webhook_test

import (
	"github.com/securesign/policy-controller-operator/cmd/internal/constants"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GeneratePolicyControllerObj(namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": constants.PolicyControllerAPIVersion,
			"kind":       constants.PolicyControllerKind,
			"metadata": map[string]interface{}{
				"name":      "policy-controller",
				"namespace": namespace,
			},
		},
	}
	return obj
}
