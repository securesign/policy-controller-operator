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
	managerPath        = "../../config/manager/manager.yaml"
	webhookSvcPath     = "../../config/webhook/service.yaml"
	metricsSvcPath     = "../../config/default/metrics_service.yaml"
	networkPolicyPath  = "../../config/network-policy/allow-metrics-traffic.yaml"
	serviceMonitorPath = "../../config/prometheus/monitor.yaml"

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
		if dep.Kind == "Deployment" && dep.Name != "" {
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

func TestDeploymentSelectorUsesUniqueLabel(t *testing.T) {
	dep := loadDeployment(t)

	sel := dep.Spec.Selector
	if sel == nil {
		t.Fatal("spec.selector is nil — must not be omitted")
	}

	want := map[string]string{uniqueLabel: uniqueLabelValue}

	if len(sel.MatchLabels) != len(want) {
		t.Errorf("spec.selector.matchLabels has %d keys, want exactly %d: got %v, want %v",
			len(sel.MatchLabels), len(want), sel.MatchLabels, want)
	}

	for key, expected := range want {
		got, ok := sel.MatchLabels[key]
		if !ok || got != expected {
			t.Errorf("spec.selector.matchLabels[%q] = %q, want %q",
				key, got, expected)
		}
	}
}

func TestDeploymentName(t *testing.T) {
	dep := loadDeployment(t)

	if dep.Name != "policy-controller-operator" {
		t.Errorf("Deployment name = %q, want %q — "+
			"renamed from controller-manager to force OLM to create a fresh resource "+
			"and avoid immutable-selector upgrade failures", dep.Name, "policy-controller-operator")
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

func TestPodTemplateDoesNotCarryLegacyLabel(t *testing.T) {
	dep := loadDeployment(t)

	if _, has := dep.Spec.Template.Labels["control-plane"]; has {
		t.Error("pod template must NOT carry the legacy control-plane label — " +
			"it causes collisions with other operators using the same default label")
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

	if _, has := svc.Spec.Selector["control-plane"]; has {
		t.Error("webhook Service selector must NOT contain the legacy control-plane label")
	}
}

func TestMetricsServiceSelectsOnlyUniqueLabel(t *testing.T) {
	svc := loadService(t, metricsSvcPath)

	if len(svc.Spec.Selector) != 1 {
		t.Errorf("metrics Service selector has %d keys, want exactly 1: %v",
			len(svc.Spec.Selector), svc.Spec.Selector)
	}

	got, ok := svc.Spec.Selector[uniqueLabel]
	if !ok || got != uniqueLabelValue {
		t.Errorf("metrics Service selector[%q] = %q, want %q",
			uniqueLabel, got, uniqueLabelValue)
	}

	if _, has := svc.Spec.Selector["control-plane"]; has {
		t.Error("metrics Service selector must NOT contain the legacy control-plane label")
	}
}

func TestLabelsConsistent(t *testing.T) {
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

func TestServiceMonitorSelectsMetricsService(t *testing.T) {
	metricsSvc := loadService(t, metricsSvcPath)
	webhookSvc := loadService(t, webhookSvcPath)

	data, err := os.ReadFile(serviceMonitorPath)
	if err != nil {
		t.Fatalf("reading %s: %v", serviceMonitorPath, err)
	}
	var raw map[string]interface{}
	if err := sigyaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decoding ServiceMonitor: %v", err)
	}
	spec, ok := raw["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("ServiceMonitor missing spec")
	}
	selector, ok := spec["selector"].(map[string]interface{})
	if !ok {
		t.Fatal("ServiceMonitor missing spec.selector")
	}
	matchLabels, ok := selector["matchLabels"].(map[string]interface{})
	if !ok {
		t.Fatal("ServiceMonitor missing spec.selector.matchLabels")
	}

	if _, ok := matchLabels["app.kubernetes.io/component"]; !ok {
		t.Error("ServiceMonitor selector must include app.kubernetes.io/component to distinguish " +
			"the metrics Service from other Services sharing the same app.kubernetes.io/name label")
	}

	for key, val := range matchLabels {
		strVal, ok := val.(string)
		if !ok {
			t.Fatalf("ServiceMonitor selector value for %q is not a string: %v", key, val)
		}
		svcVal := metricsSvc.Labels[key]
		if svcVal != strVal {
			t.Errorf("ServiceMonitor selector %s=%s not satisfied by metrics Service labels (has %s=%s)",
				key, strVal, key, svcVal)
		}
	}

	allMatch := true
	for key, val := range matchLabels {
		strVal, _ := val.(string)
		if webhookSvc.Labels[key] != strVal {
			allMatch = false
			break
		}
	}
	if allMatch {
		t.Error("ServiceMonitor selector must not match the webhook Service — " +
			"add a distinguishing label like app.kubernetes.io/component to the metrics Service")
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

	if _, has := podSel["control-plane"]; has {
		t.Error("NetworkPolicy podSelector must NOT use the legacy control-plane label")
	}
}
