package test

import (
	"encoding/json"

	"github.com/securesign/policy-controller-operator/cmd/webhook"
	admission "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func generateAdmissionReviewResource(namespace string) admission.AdmissionReview {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   webhook.PolicyControllerGroup,
			Version: webhook.PolicyControllerVersion,
			Kind:    webhook.PolicyControllerResource,
		})
	obj.SetName("example")
	obj.SetNamespace(namespace)
	raw, _ := json.Marshal(obj)

	return admission.AdmissionReview{
		Request: &admission.AdmissionRequest{
			UID: "12345",
			Kind: metav1.GroupVersionKind{
				Group:   webhook.PolicyControllerGroup,
				Version: webhook.PolicyControllerVersion,
				Kind:    webhook.PolicyControllerResource,
			},
			Resource: metav1.GroupVersionResource{
				Group:    webhook.PolicyControllerGroup,
				Version:  webhook.PolicyControllerVersion,
				Resource: webhook.PolicyControllerResource,
			},
			Operation: admission.Create,
			Object:    runtime.RawExtension{Raw: raw},
			Namespace: namespace,
			Name:      "example",
		},
	}
}
