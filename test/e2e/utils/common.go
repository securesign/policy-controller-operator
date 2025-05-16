package e2e_utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultFulcioUrl = "http://fulcio-server.local"
	fulcioEnv        = "FULCIO_URL"

	defaultRekorUrl = "http://rekor-server.local"
	rekorEnv        = "REKOR_URL"

	testImageEnv = "TEST_IMAGE"
)

func EnvOrDefault(env string, defualt string) string {
	val, ok := os.LookupEnv(env)
	if ok {
		return val
	}
	return defualt
}

func FulcioUrl() string {
	return EnvOrDefault(fulcioEnv, defaultFulcioUrl)
}

func RekorUrl() string {
	return EnvOrDefault(rekorEnv, defaultRekorUrl)
}

func TestImage() string {
	return EnvOrDefault(testImageEnv, "")
}

func ExpectExists(name, namespace string, obj client.Object, k8sClient client.Client, ctx context.Context) {
	Eventually(func() error {
		return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj)
	}, "10s", "1s").Should(Succeed(), "expected %T %q to exist", obj, name)
}

func VerifyByCosign(ctx context.Context, targetImageName string) {
	oidcToken, err := OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	Expect(Execute("cosign", "initialize", "--mirror="+TufUrl(), "--root="+TufUrl()+"/root.json")).To(Succeed())
	Expect(Execute("cosign", "sign", "-y", "--fulcio-url="+FulcioUrl(), "--rekor-url="+RekorUrl(), "--oidc-issuer="+OidcIssuerUrl(), "--oidc-client-id="+OidcClientID(), "--identity-token="+oidcToken, targetImageName)).To(Succeed())
	Expect(Execute("cosign", "verify", "--rekor-url="+RekorUrl(), "--certificate-identity-regexp", ".*@redhat", "--certificate-oidc-issuer-regexp", ".*keycloak.*", targetImageName)).To(Succeed())
}

func RenderTemplate(path string, data interface{}) ([]byte, error) {
	tpl, err := template.ParseFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", path, err)
	}

	return buf.Bytes(), nil
}
