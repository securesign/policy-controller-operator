apiVersion: policy.sigstore.dev/v1alpha1
kind: TrustRoot
metadata:
  name: {{ .TRUST_ROOT_NAME }}
spec:
  repository:
    root: |-
      {{ .TUFRoot }}
    mirrorFS: |-
      {{ .REPOSITORY }}
