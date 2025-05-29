package webhook

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	admission "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"encoding/json"
)

const (
	PolicyControllerGroup     = "rhtas.charts.redhat.com"
	PolicyControllerVersion   = "v1alpha1"
	PolicyControllerResource  = "policycontrollers"
	PolicyControllerInstallNs = "policy-controller-operator"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecFactory  = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecFactory.UniversalDeserializer()
)

func init() {
	_ = admission.AddToScheme(runtimeScheme)
}

func serve(w http.ResponseWriter, r *http.Request, admit func(admission.AdmissionReview) *admission.AdmissionResponse) {
	data, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		log.Error().Err(err).Msg("failed to read request body")
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		log.Error().Msgf("contentType=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect application/json", http.StatusUnsupportedMediaType)
		return
	}

	log.Info().Msgf("handling request: %s", data)
	obj, gvk, err := deserializer.Decode(data, nil, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to decode AdmissionReview")
		http.Error(w, "cannot decode AdmissionReview", http.StatusBadRequest)
		return
	}

	requestedAdmissionReview, ok := obj.(*admission.AdmissionReview)
	if !ok {
		log.Error().Msgf("unexpected type %T, expected *admission.AdmissionReview", obj)
		http.Error(w, "unexpected object type", http.StatusBadRequest)
		return
	}

	responseAdmissionReview := &admission.AdmissionReview{}
	responseAdmissionReview.SetGroupVersionKind(*gvk)
	responseAdmissionReview.Response = admit(*requestedAdmissionReview)
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	log.Info().Msgf("sending response: %v", responseAdmissionReview)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responseAdmissionReview); err != nil {
		log.Err(err).Msg("failed to encode AdmissionReview response")
	}
}

func ServeValidate(w http.ResponseWriter, r *http.Request) {
	serve(w, r, Validate)
}

func Validate(ar admission.AdmissionReview) *admission.AdmissionResponse {
	log.Info().Msgf("validating policy controller resource")
	expectedRes := metav1.GroupVersionResource{
		Group:    PolicyControllerGroup,
		Version:  PolicyControllerVersion,
		Resource: PolicyControllerResource,
	}
	if ar.Request.Resource != expectedRes {
		log.Error().Msgf("expected resource: %s, got: %s", PolicyControllerResource, ar.Request.Resource.String())
		return &admission.AdmissionResponse{Allowed: false}
	}

	var obj unstructured.Unstructured
	if err := json.Unmarshal(ar.Request.Object.Raw, &obj); err != nil {
		return &admission.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: fmt.Sprintf("error parsing policy controller resource: %v", err),
			},
		}
	}

	if obj.GetNamespace() != PolicyControllerInstallNs {
		return &admission.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: fmt.Sprintf("%s may only be created in the '%s' namespace", PolicyControllerResource, PolicyControllerInstallNs),
			},
		}
	}
	return &admission.AdmissionResponse{Allowed: true}
}
