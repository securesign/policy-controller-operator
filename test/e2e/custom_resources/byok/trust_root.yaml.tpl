apiVersion: policy.sigstore.dev/v1alpha1
kind: TrustRoot
metadata:
  name: byok-install-trust-root
spec:
  sigstoreKeys:
    certificateAuthorities:
    - subject:
        organization: {{ .FULCIO_ORG_NAME }}
        commonName: {{ .FULCIO_COMMON_NAME }}
      uri: {{ .FULCIO_URL }}
      certChain: |-
{{ nindent 8 .FULCIO_CERT_CHAIN }}
    ctLogs:
    - baseURL: {{ .CTLOG_URL }}
      hashAlgorithm: {{ .CTLOG_HASH_ALGORITHM }}
      publicKey: |-
{{ nindent 8 .CTFE_PUBLIC_KEY }}
    tLogs:
    - baseURL: {{ .REKOR_URL }}
      hashAlgorithm: {{ .REKOR_HASH_ALGORITHM }}
      publicKey: |-
{{ nindent 8 .REKOR_PUBLIC_KEY }}
    timestampAuthorities:
    - subject:
        organization: {{ .TSA_ORG_NAME }}
        commonName: {{ .TSA_COMMON_NAME }}
      uri: {{ .TSA_URL }}
      certChain: |-
{{ nindent 8 .TSA_CERT_CHAIN }}
