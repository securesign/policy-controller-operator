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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InstallNamespace         = "policy-controller-operator"
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
