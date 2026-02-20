package e2e_utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SubscriptionPath               = "../utils/custom_resources/subscription/subscription.yaml.tpl"
	CatalogSourcePath              = "../utils/custom_resources/catalog_source/catalog_source.yaml.tpl"
	OperatorGroupPath              = "../utils/custom_resources/operator_group/operator_group.yaml.tpl"
	PolicyControllerCRPath         = "../utils/custom_resources/policy_controller/common_policy_controller.yaml.tpl"
	TrustRootCommonCrPath          = "../utils/custom_resources/trust_roots/common_trust_root.yaml.tpl"
	ClusterimagepolicyCommonCrPath = "../utils/custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"

	TrustRootBYOKCrPath          = "../utils/custom_resources/trust_roots/byok_trust_root.yaml.tpl"
	ClusterimagepolicyBYOKCrPath = "../utils/custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"

	TrustRootSTUFCrPath          = "../utils/custom_resources/trust_roots/stuf_trust_root.yaml.tpl"
	ClusterimagepolicySTUFCrPath = "../utils/custom_resources/cluster_image_policies/common_cluster_image_policy.yaml.tpl"

	InstallNamespace         = "policy-controller-operator"
	PackageName              = "policy-controller-operator"
	DeploymentName           = "policycontroller-sample-policy-controller-webhook"
	ValidatingWebhookName    = "policy.rhtas.com"
	MutatingWebhookName      = "policy.rhtas.com"
	CipValidatingWebhookName = "validating.clusterimagepolicy.rhtas.com"
	CipMutatingWebhookName   = "defaulting.clusterimagepolicy.rhtas.com"
	WebhookSvc               = "webhook"
	MetricsSvc               = "policycontroller-sample-policy-controller-webhook-metrics"
	SecretName               = "webhook-certs"
)

func EnvOrDefault(env string, defualt string) string {
	val, ok := os.LookupEnv(env)
	if ok {
		return val
	}
	return defualt
}

func ExpectExists(name, namespace string, obj client.Object, k8sClient client.Client, ctx context.Context) {
	Eventually(func() error {
		return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj)
	}).WithContext(ctx).Should(Succeed(), "expected %T %q to exist", obj, name)
}

func RenderTemplate(path string, data interface{}) ([]byte, error) {
	funcMap := template.FuncMap{"nindent": nindent}
	tpl, err := template.New(filepath.Base(path)).Funcs(funcMap).ParseFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", path, err)
	}

	return buf.Bytes(), nil
}

func nindent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n"+pad)
}

func Base64EncodeString(src []byte) string {
	return base64.StdEncoding.EncodeToString(src)
}

func GetNestedString(obj map[string]any, path ...string) (string, error) {
	val, found, err := unstructured.NestedString(obj, path...)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s not present yet", strings.Join(path, "."))
	}
	if val == "" {
		return "", fmt.Errorf("%s is empty", strings.Join(path, "."))
	}
	return val, nil
}
