package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/securesign/policy-controller-operator/cmd/webhook"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name               string
		namespace          string
		allowed            bool
		unexpectedResource bool
	}{
		{
			name:               "correct namespace",
			namespace:          webhook.PolicyControllerInstallNs,
			allowed:            true,
			unexpectedResource: false,
		},
		{
			name:               "wrong namespace",
			namespace:          "default",
			allowed:            false,
			unexpectedResource: false,
		},
		{
			name:               "unexpected resources",
			namespace:          webhook.PolicyControllerInstallNs,
			allowed:            false,
			unexpectedResource: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := generateAdmissionReviewResource(tt.namespace, tt.unexpectedResource)
			resp := webhook.Validate(ar)
			if resp.Allowed != tt.allowed {
				t.Fatalf("expected Allowed=%v, got %v (message: %v)",
					tt.allowed, resp.Allowed, resp.Result)
			}
		})
	}
}

func TestServeValidate(t *testing.T) {
	admissionReview := generateAdmissionReviewResource(webhook.PolicyControllerInstallNs, false)
	body, err := json.Marshal(admissionReview)
	if err != nil {
		t.Fatalf("failed to marshal admission review: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(webhook.ServeValidate)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	req = httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Errorf("expected non-200 status for invalid content-type, got %d", rr.Code)
	}
}
