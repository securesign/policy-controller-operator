package e2e_utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

var (
	cosignV3Once sync.Once
	cosignV3     bool
)

func IsCosignV3() bool {
	cosignV3Once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "cosign", "version").CombinedOutput()
		if err != nil {
			fmt.Fprintf(core.GinkgoWriter, "WARNING: could not determine cosign version, defaulting to v2: %v\n", err)
			return
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "GitVersion:") {
				version := strings.TrimSpace(strings.TrimPrefix(line, "GitVersion:"))
				version = strings.TrimPrefix(version, "v")
				cosignV3 = strings.HasPrefix(version, "3.")
				return
			}
		}
		fmt.Fprintf(core.GinkgoWriter, "WARNING: could not parse cosign version from output, defaulting to v2\n")
	})
	return cosignV3
}

func cosignSignArgs(bundleFormat bool) []string {
	if bundleFormat {
		return []string{"--new-bundle-format"}
	}
	if IsCosignV3() {
		return []string{"--new-bundle-format=false", "--use-signing-config=false"}
	}
	return nil
}

// signingConfigEnv returns a copy of the process environment with COSIGN_*
// service-URL variables removed so that cosign v3 uses its TUF-based signing
// config instead. SIGSTORE_ID_TOKEN is injected for authentication.
func filteredCosignEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "COSIGN_FULCIO_URL", "COSIGN_REKOR_URL", "COSIGN_TIMESTAMP_SERVER_URL",
			"COSIGN_OIDC_ISSUER", "COSIGN_OIDC_CLIENT_ID", "COSIGN_REPOSITORY",
			"COSIGN_IDENTITY_TOKEN":
			continue
		}
		env = append(env, e)
	}
	return env
}

func signingConfigEnv(oidcToken string) []string {
	return append(filteredCosignEnv(), "SIGSTORE_ID_TOKEN="+oidcToken)
}

func VerifyByCosign(ctx context.Context, targetImageName string, bundleFormat bool) {
	Eventually(func() error {
		return Execute("cosign", "initialize", "--mirror="+TufUrl(), "--root="+TufUrl()+"/root.json")
	}).WithContext(ctx).Should(Succeed())

	Eventually(func() error {
		oidcToken, err := OidcToken(ctx)
		if err != nil {
			return fmt.Errorf("fetching OIDC token: %w", err)
		}

		if bundleFormat && IsCosignV3() {
			return ExecuteWithEnv(signingConfigEnv(oidcToken), "cosign", "sign", "-y", targetImageName)
		}

		signArgs := []string{"sign", "-y"}
		signArgs = append(signArgs, cosignSignArgs(bundleFormat)...)
		signArgs = append(signArgs,
			"--timestamp-server-url="+TsaUrl(),
			"--fulcio-url="+FulcioUrl(),
			"--rekor-url="+RekorUrl(),
			"--oidc-issuer="+OidcIssuerUrl(),
			"--oidc-client-id="+OidcClientID(),
			"--identity-token="+oidcToken,
			targetImageName,
		)
		return Execute("cosign", signArgs...)
	}).WithContext(ctx).Should(Succeed())

	Eventually(func() error {
		verifyArgs := []string{"verify",
			"--rekor-url=" + RekorUrl(),
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			targetImageName,
		}
		if bundleFormat && IsCosignV3() {
			return ExecuteWithEnv(filteredCosignEnv(), "cosign", verifyArgs...)
		}
		return Execute("cosign", verifyArgs...)
	}).WithContext(ctx).Should(Succeed())
}

func cosignAttest(ctx context.Context, targetImageName string, bundleFormat bool, attestType, predicate string) {
	Eventually(func() error {
		oidcToken, err := OidcToken(ctx)
		if err != nil {
			return fmt.Errorf("fetching OIDC token: %w", err)
		}

		if bundleFormat && IsCosignV3() {
			return ExecuteWithEnvAndInput(signingConfigEnv(oidcToken), predicate, "cosign",
				"attest", "--yes",
				"--predicate", "-",
				"--type", attestType,
				targetImageName,
			)
		}

		attestArgs := []string{
			"attest",
			"--yes",
		}
		attestArgs = append(attestArgs, cosignSignArgs(bundleFormat)...)
		attestArgs = append(attestArgs,
			"--predicate", "-",
			"--type", attestType,
			"--fulcio-url="+FulcioUrl(),
			"--rekor-url="+RekorUrl(),
			"--oidc-issuer="+OidcIssuerUrl(),
			"--oidc-client-id="+OidcClientID(),
			"--identity-token="+oidcToken,
		)
		if !IsCosignV3() {
			attestArgs = append(attestArgs, "--timestamp-server-url="+TsaUrl())
		}
		attestArgs = append(attestArgs, targetImageName)
		return ExecuteWithInput(predicate, "cosign", attestArgs...)
	}).WithContext(ctx).Should(Succeed())
}

func AttachProvenance(ctx context.Context, targetImageName string, bundleFormat bool) {
	const provenance = `{
	  "buildType": "https://example.com/e2e-test",
	  "builder":   { "id": "e2e-test" }
	}`
	cosignAttest(ctx, targetImageName, bundleFormat, "slsaprovenance", provenance)
}

func AttachSBOM(ctx context.Context, targetImageName string, bundleFormat bool) {
	sbom := fmt.Sprintf(`{
	  "$schema":"http://cyclonedx.org/schema/bom-1.6.schema.json",
	  "bomFormat":"CycloneDX",
	  "specVersion":"1.6",
	  "version":1,
	  "metadata":{
	    "component":{
	      "type":"container",
	      "name":"%s"
	    }
	  }
	}`, targetImageName)
	cosignAttest(ctx, targetImageName, bundleFormat, "cyclonedx", sbom)
}
