package e2e_utils

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
)

func VerifyByCosign(ctx context.Context, targetImageName string) {
	oidcToken, err := OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	Expect(Execute("cosign", "initialize", "--mirror="+TufUrl(), "--root="+TufUrl()+"/root.json")).To(Succeed())
	Expect(Execute("cosign", "sign", "-y", "--fulcio-url="+FulcioUrl(), "--rekor-url="+RekorUrl(), "--oidc-issuer="+OidcIssuerUrl(), "--oidc-client-id="+OidcClientID(), "--identity-token="+oidcToken, targetImageName)).To(Succeed())
	Expect(Execute("cosign", "verify", "--rekor-url="+RekorUrl(), "--certificate-identity-regexp", ".*@redhat", "--certificate-oidc-issuer-regexp", ".*keycloak.*", targetImageName)).To(Succeed())
}

func AttachProvenance(ctx context.Context, targetImageName string) {
	oidcToken, err := OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	const provenance = `{
	  "buildType": "https://example.com/e2e-test",
	  "builder":   { "id": "e2e-test" }
	}`

	err = ExecuteWithInput(
		provenance,
		"cosign", "attest",
		"--yes",
		"--predicate", "-",
		"--type", "slsaprovenance",
		"--fulcio-url="+FulcioUrl(),
		"--rekor-url="+RekorUrl(),
		"--oidc-issuer="+OidcIssuerUrl(),
		"--oidc-client-id="+OidcClientID(),
		"--identity-token="+oidcToken,
		targetImageName,
	)
	Expect(err).To(Succeed())
}

func AttachSBOM(ctx context.Context, targetImageName string) {
	oidcToken, err := OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

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

	err = ExecuteWithInput(
		sbom,
		"cosign", "attest",
		"--yes",
		"--predicate", "-",
		"--type", "cyclonedx",
		"--fulcio-url="+FulcioUrl(),
		"--rekor-url="+RekorUrl(),
		"--oidc-issuer="+OidcIssuerUrl(),
		"--oidc-client-id="+OidcClientID(),
		"--identity-token="+oidcToken,
		targetImageName,
	)
	Expect(err).To(Succeed())
}
