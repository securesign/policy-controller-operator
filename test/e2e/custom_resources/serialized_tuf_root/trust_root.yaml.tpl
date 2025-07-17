apiVersion: policy.sigstore.dev/v1alpha1
kind: TrustRoot
metadata:
  name: serialized-tuf-install-trust-root
spec:
  repository:
    root: |-
      {{ .TUFRoot }}
    mirrorFS: |-
      {{ .REPOSITORY }}
