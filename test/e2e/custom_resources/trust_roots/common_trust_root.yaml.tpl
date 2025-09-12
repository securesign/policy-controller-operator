apiVersion: policy.sigstore.dev/v1alpha1
kind: TrustRoot
metadata:
  name: common-install-trust-root
spec:
  remote:
    mirror: {{ .TUFMirror }}
    root: | 
      {{ .TUFRoot }}
