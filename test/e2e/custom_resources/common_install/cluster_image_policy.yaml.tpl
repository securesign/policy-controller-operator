apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: common-install-cluster-image-policy
spec:
  images:
    - glob: "**"
  authorities:
    - keyless:
        url: {{ .FULCIO_URL }}
        trustRootRef: common-install-trust-root
        identities:
          - issuer: {{ .OIDC_ISSUER_URL }}
            subject: {{ .OIDC_ISSUER_SUBJECT }}
      ctlog:
        url: {{ .REKOR_URL }}
        trustRootRef: common-install-trust-root
