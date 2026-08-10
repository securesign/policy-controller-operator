package e2e_utils

import (
	"context"
	"fmt"
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

func cosignSignArgs() []string {
	if IsCosignV3() {
		return []string{"--new-bundle-format=false", "--use-signing-config=false"}
	}
	return nil
}

func VerifyByCosign(ctx context.Context, targetImageName string) {
	Eventually(func() error {
		return Execute("cosign", "initialize", "--mirror="+TufUrl(), "--root="+TufUrl()+"/root.json")
	}).WithContext(ctx).Should(Succeed())

	Eventually(func() error {
		oidcToken, err := OidcToken(ctx)
		if err != nil {
			return fmt.Errorf("fetching OIDC token: %w", err)
		}

		signArgs := []string{"sign", "-y"}
		signArgs = append(signArgs, cosignSignArgs()...)
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
		return Execute("cosign", "verify",
			"--rekor-url="+RekorUrl(),
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			targetImageName,
		)
	}).WithContext(ctx).Should(Succeed())
}

func AttachProvenance(ctx context.Context, targetImageName string) {
	const provenance = `{
	  "buildType": "https://example.com/e2e-test",
	  "builder":   { "id": "e2e-test" }
	}`

	Eventually(func() error {
		oidcToken, err := OidcToken(ctx)
		if err != nil {
			return fmt.Errorf("fetching OIDC token: %w", err)
		}

		attestArgs := []string{
			"attest",
			"--yes",
		}
		attestArgs = append(attestArgs, cosignSignArgs()...)
		attestArgs = append(attestArgs,
			"--predicate", "-",
			"--type", "slsaprovenance",
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
		return ExecuteWithInput(provenance, "cosign", attestArgs...)
	}).WithContext(ctx).Should(Succeed())
}

func AttachSBOM(ctx context.Context, targetImageName string) {
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

	Eventually(func() error {
		oidcToken, err := OidcToken(ctx)
		if err != nil {
			return fmt.Errorf("fetching OIDC token: %w", err)
		}

		attestArgs := []string{
			"attest",
			"--yes",
		}
		attestArgs = append(attestArgs, cosignSignArgs()...)
		attestArgs = append(attestArgs,
			"--predicate", "-",
			"--type", "cyclonedx",
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
		return ExecuteWithInput(sbom, "cosign", attestArgs...)
	}).WithContext(ctx).Should(Succeed())
}
