# Configuring the Policy Controller Cluster Image Policy resource
This document will guide you through configuring a basic Policy Controller Cluster Image Policy resource for a Red Hat Trusted Artifact Signer (RHTAS) instance.

## Prerequisites
Before proceeding, ensure you have the following:
- A running RHTAS instance (Red Hat Trusted Artifact Signer)
- A running policy controller operator instance
- A running Policy Controller instance
- Required CLI tools installed:
    - oc

## Configuring a basic Cluster Image Policy resource
1. Grab Fulcio & Rekor URLs from your RHTAS install
    ```
    export RHTAS_INSTALL_NAMESPACE=<rhtas-install-namespace>
    export FULCIO_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get fulcio -o jsonpath='{.items[0].status.url}')"
    export REKOR_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get rekor -o jsonpath='{.items[0].status.url}')"
    ```

3. Set OIDC issuer & subject
    ```
    export OIDC_ISSUER_URL="https://<your-issuer>"
    export OIDC_SUBJECT="<subject>"
    ```

4. Set the TrustRoot reference
    ```
    export TRUST_ROOT_REF="<trustroot-name>"
    ```

1. Create or Apply the Cluster Image Policy (Cluster-Scoped)
    ```sh
    cat <<EOF | kubectl apply -f -
    apiVersion: policy.sigstore.dev/v1beta1
    kind: ClusterImagePolicy
    metadata:
      name: cluster-image-policy
    spec:
      images:
        - glob: "**"
      authorities:
        - keyless:
            url: $FULCIO_URL
            trustRootRef: $TRUST_ROOT_REF
            identities:
              - issuer: $OIDC_ISSUER_URL
                subject: $OIDC_SUBJECT
          ctlog:
            url: $REKOR_URL
            trustRootRef: $TRUST_ROOT_REF
          rfc3161timestamp:
            trustRootRef: $TRUST_ROOT_REF
    EOF
    ```

    NOTES:
    * images[*].glob of ** means “evaluate all images.”

For more configuration options please visit the upstream documentation: https://docs.sigstore.dev/policy-controller/overview/
