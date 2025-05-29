package test

import (
	"testing"

	"github.com/securesign/policy-controller-operator/cmd/webhook"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		allowed   bool
	}{
		{
			name:      "correct namespace",
			namespace: webhook.PolicyControllerInstallNs,
			allowed:   true,
		},
		{
			name:      "wrong namespace",
			namespace: "default",
			allowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := generateAdmissionReviewResource(tt.namespace)
			resp := webhook.Validate(ar)

			if resp.Allowed != tt.allowed {
				t.Fatalf("expected Allowed=%v, got %v (message: %v)",
					tt.allowed, resp.Allowed, resp.Result)
			}
		})
	}
}
