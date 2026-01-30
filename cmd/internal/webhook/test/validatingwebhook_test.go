package webhook_test

import (
	"context"
	"testing"

	"github.com/securesign/policy-controller-operator/cmd/internal/constants"
	"github.com/securesign/policy-controller-operator/cmd/internal/webhook"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPolicyControllerValidator(t *testing.T) {
	validator := webhook.PolicyControllerValidator{}
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		expectErr bool
	}{
		{
			name:      "correct namespace",
			obj:       GeneratePolicyControllerObj(constants.PolicyControllerInstallNs),
			expectErr: false,
		},
		{
			name:      "wrong namespace",
			obj:       GeneratePolicyControllerObj("default"),
			expectErr: true,
		},
		{
			name:      "wrong resource type",
			obj:       &unstructured.Unstructured{},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, createErr := validator.ValidateCreate(context.Background(), tc.obj)
			if tc.expectErr {
				require.Error(t, createErr)
			} else {
				require.NoError(t, createErr)
			}

			_, updateErr := validator.ValidateUpdate(context.Background(), tc.obj, tc.obj)
			if tc.expectErr {
				require.Error(t, updateErr)
			} else {
				require.NoError(t, updateErr)
			}

			_, deleteErr := validator.ValidateDelete(context.Background(), tc.obj)
			require.NoError(t, deleteErr)
		})
	}
}
