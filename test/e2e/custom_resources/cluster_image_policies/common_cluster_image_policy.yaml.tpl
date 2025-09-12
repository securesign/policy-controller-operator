apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: {{ .CIP_NAME }}
spec:
  images:
    - glob: "**"
  authorities:
    - keyless:
        url: {{ .FULCIO_URL }}
        trustRootRef: {{ .TRUST_ROOT_REF }}
        identities:
          - issuer: {{ .OIDC_ISSUER_URL }}
            subject: {{ .OIDC_ISSUER_SUBJECT }}
      ctlog:
        url: {{ .REKOR_URL }}
        trustRootRef: {{ .TRUST_ROOT_REF }}
      attestations:
        - name: match-sbom
          predicateType: https://cyclonedx.org/bom
          policy:
            type: cue
            data: |
              predicate: {
                metadata: {
                  component: {
                    name: "{{ .TEST_IMAGE }}"
                  }
                }
              }
        - name: provenance-check
          predicateType: https://slsa.dev/provenance/v0.2
          policy:
            type: cue
            data: |
              predicate: {
                builder: {
                  id: "e2e-test"
                }
              }
