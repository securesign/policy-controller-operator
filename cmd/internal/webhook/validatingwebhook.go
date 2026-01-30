package webhook

import (
	"context"
	"fmt"

	"github.com/securesign/policy-controller-operator/cmd/internal/constants"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate,mutating=false,failurePolicy=fail,groups=rhtas.charts.redhat.com,resources=policycontrollers,verbs=create,versions=v1alpha1,name=policycontrollers.rhtas.charts.redhat.com
// PolicyControllerValidator validates PolicyControllerResources
type PolicyControllerValidator struct{}

// validate validates PolicyControllerResources namespace
func (v *PolicyControllerValidator) validate(ctx context.Context, obj *unstructured.Unstructured) (admission.Warnings, error) {
	log := logf.FromContext(ctx)

	if ns := obj.GetNamespace(); ns != constants.PolicyControllerInstallNs {
		err := fmt.Errorf("%s objects may only be created in the %q namespace (got %q)", obj.GetKind(), constants.PolicyControllerInstallNs, ns)
		log.Info("denying creation: wrong namespace", "namespace", ns)
		return nil, err
	}

	return nil, nil
}

func (v *PolicyControllerValidator) ValidateCreate(ctx context.Context, obj *unstructured.Unstructured) (admission.Warnings, error) {
	return v.validate(ctx, obj)
}

func (v *PolicyControllerValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *unstructured.Unstructured) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *PolicyControllerValidator) ValidateDelete(ctx context.Context, obj *unstructured.Unstructured) (admission.Warnings, error) {
	// Allow all delete operations
	return nil, nil
}
