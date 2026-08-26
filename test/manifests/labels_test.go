package manifests_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigyaml "sigs.k8s.io/yaml"
)

const (
	managerPath       = "../../config/manager/manager.yaml"
	webhookSvcPath    = "../../config/webhook/service.yaml"
	metricsSvcPath    = "../../config/default/metrics_service.yaml"
	networkPolicyPath = "../../config/network-policy/allow-metrics-traffic.yaml"

	legacySelectorLabel = "control-plane"
	legacySelectorValue = "controller-manager"

	uniqueLabel      = "app.kubernetes.io/name"
	uniqueLabelValue = "policy-controller-operator"
)

func loadDeployment(t *testing.T) *appsv1.Deployment {
	t.Helper()
	data, err := os.ReadFile(managerPath)
	if err != nil {
		t.Fatalf("reading %s: %v", managerPath, err)
	}

	reader := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		var dep appsv1.Deployment
		err := reader.Decode(&dep)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decoding deployment: %v", err)
		}
		if dep.Kind == "Deployment" && dep.Name == "controller-manager" {
			return &dep
		}
	}
	t.Fatal("no Deployment found in manager.yaml")
	return nil
}

func loadService(t *testing.T, path string) *corev1.Service {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var svc corev1.Service
	if err := sigyaml.Unmarshal(data, &svc); err != nil {
		t.Fatalf("decoding service from %s: %v", path, err)
	}
	return &svc
}

func TestDeploymentSelectorCompatibleWithV102(t *testing.T) {
	dep := loadDeployment(t)

	sel := dep.Spec.Selector
	if sel == nil {
		t.Fatal("spec.selector is nil — must not be omitted")
	}

	want := map[string]string{legacySelectorLabel: legacySelectorValue}

	if len(sel.MatchLabels) != len(want) {
		t.Errorf("spec.selector.matchLabels has %d keys, want exactly %d: got %v, want %v",
			len(sel.MatchLabels), len(want), sel.MatchLabels, want)
	}

	for key, expected := range want {
		got, ok := sel.MatchLabels[key]
		if !ok || got != expected {
			t.Errorf("spec.selector.matchLabels[%q] = %q, want %q (v1.0.2 compat)",
				key, got, expected)
		}
	}
}

func TestPodTemplateMatchesSelector(t *testing.T) {
	dep := loadDeployment(t)

	podLabels := dep.Spec.Template.Labels
	got, ok := podLabels[legacySelectorLabel]
	if !ok || got != legacySelectorValue {
		t.Errorf("pod template labels[%q] = %q, want %q",
			legacySelectorLabel, got, legacySelectorValue)
	}
}

func TestPodTemplateCarriesUniqueLabel(t *testing.T) {
	dep := loadDeployment(t)

	podLabels := dep.Spec.Template.Labels
	got, ok := podLabels[uniqueLabel]
	if !ok || got != uniqueLabelValue {
		t.Errorf("pod template labels[%q] = %q, want %q",
			uniqueLabel, got, uniqueLabelValue)
	}
}

func TestWebhookServiceSelectsOnlyUniqueLabel(t *testing.T) {
	svc := loadService(t, webhookSvcPath)

	if len(svc.Spec.Selector) != 1 {
		t.Errorf("webhook Service selector has %d keys, want exactly 1: %v",
			len(svc.Spec.Selector), svc.Spec.Selector)
	}

	got, ok := svc.Spec.Selector[uniqueLabel]
	if !ok || got != uniqueLabelValue {
		t.Errorf("webhook Service selector[%q] = %q, want %q",
			uniqueLabel, got, uniqueLabelValue)
	}

	if _, has := svc.Spec.Selector[legacySelectorLabel]; has {
		t.Error("webhook Service selector must NOT contain the legacy control-plane label")
	}
}

func TestMetricsServiceSelectsOnlyUniqueLabel(t *testing.T) {
	svc := loadService(t, metricsSvcPath)

	got, ok := svc.Spec.Selector[uniqueLabel]
	if !ok || got != uniqueLabelValue {
		t.Errorf("metrics Service selector[%q] = %q, want %q",
			uniqueLabel, got, uniqueLabelValue)
	}

	if _, has := svc.Spec.Selector[legacySelectorLabel]; has {
		t.Error("metrics Service selector must NOT contain the legacy control-plane label")
	}
}

func TestWebhookServiceIsolatesFromCollidingOperators(t *testing.T) {
	dep := loadDeployment(t)
	svc := loadService(t, webhookSvcPath)

	for key, val := range svc.Spec.Selector {
		podVal, ok := dep.Spec.Template.Labels[key]
		if !ok || podVal != val {
			t.Errorf("Service selector %s=%s does not match pod template label %s=%s",
				key, val, key, podVal)
		}
	}

	if _, has := svc.Spec.Selector[legacySelectorLabel]; has {
		t.Error("webhook Service must not select on control-plane label — " +
			"another operator sharing the namespace with control-plane=controller-manager " +
			"would receive webhook traffic")
	}
}

func TestFreshInstallLabelsConsistent(t *testing.T) {
	dep := loadDeployment(t)
	webhookSvc := loadService(t, webhookSvcPath)
	metricsSvc := loadService(t, metricsSvcPath)

	podLabels := dep.Spec.Template.Labels

	for key, val := range webhookSvc.Spec.Selector {
		if podLabels[key] != val {
			t.Errorf("webhook Service selector %s=%s not satisfied by pod template (has %s=%s)",
				key, val, key, podLabels[key])
		}
	}

	for key, val := range metricsSvc.Spec.Selector {
		if podLabels[key] != val {
			t.Errorf("metrics Service selector %s=%s not satisfied by pod template (has %s=%s)",
				key, val, key, podLabels[key])
		}
	}

	selectorLabels := dep.Spec.Selector.MatchLabels
	for key, val := range selectorLabels {
		if podLabels[key] != val {
			t.Errorf("Deployment selector %s=%s not satisfied by pod template (has %s=%s)",
				key, val, key, podLabels[key])
		}
	}
}

func TestOLMUpgradeV102ToV110SelectorUnchanged(t *testing.T) {
	dep := loadDeployment(t)

	v102Selector := map[string]string{
		"control-plane": "controller-manager",
	}

	if len(dep.Spec.Selector.MatchLabels) != len(v102Selector) {
		t.Errorf("v1.1.0 Deployment selector has %d keys, want exactly %d: got %v, want %v — extra keys break the immutable-selector upgrade path",
			len(dep.Spec.Selector.MatchLabels), len(v102Selector), dep.Spec.Selector.MatchLabels, v102Selector)
	}

	for key, expected := range v102Selector {
		got, ok := dep.Spec.Selector.MatchLabels[key]
		if !ok {
			t.Errorf("v1.1.0 Deployment selector missing key %q that v1.0.2 had", key)
			continue
		}
		if got != expected {
			t.Errorf("v1.1.0 Deployment selector[%q] = %q, v1.0.2 had %q — OLM upgrade will fail (immutable field)",
				key, got, expected)
		}
	}
}

func loadNetworkPolicy(t *testing.T) *networkingv1.NetworkPolicy {
	t.Helper()
	data, err := os.ReadFile(networkPolicyPath)
	if err != nil {
		t.Fatalf("reading %s: %v", networkPolicyPath, err)
	}
	var np networkingv1.NetworkPolicy
	if err := sigyaml.Unmarshal(data, &np); err != nil {
		t.Fatalf("decoding NetworkPolicy from %s: %v", networkPolicyPath, err)
	}
	return &np
}

func TestNetworkPolicySelectsUniqueLabel(t *testing.T) {
	np := loadNetworkPolicy(t)

	podSel := np.Spec.PodSelector.MatchLabels
	got, ok := podSel[uniqueLabel]
	if !ok || got != uniqueLabelValue {
		t.Errorf("NetworkPolicy podSelector.matchLabels[%q] = %q, want %q",
			uniqueLabel, got, uniqueLabelValue)
	}

	if _, has := podSel[legacySelectorLabel]; has {
		t.Error("NetworkPolicy podSelector must NOT use the legacy control-plane label")
	}
}
