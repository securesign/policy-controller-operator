# Configuring the Policy Controller TrustRoot resource
This document will guide you through configuring the Policy Controller TrustRoot resource for a Red Hat Trusted Artifact Signer (RHTAS) instance.
It covers three scenarios:
- TrustRoot for a Custom TUF Repository
- TrustRoot for “Bring Your Own Keys”
- TrustRoot for a Serialized TUF Root

## Prerequisites
Before proceeding, ensure you have the following:
- A running RHTAS instance (Red Hat Trusted Artifact Signer)
- A running policy controller operator instance
- A running Policy Controller instance
- Required CLI tools installed:
    - oc
    - curl
    - tuftool

## Configuring TrustRoot for custom TUF repository
1. Retrieve the TUF Mirror URL  
    Get the TUF mirror URL from the RHTAS TUF resource.
    ```sh
    export RHTAS_INSTALL_NAMESPACE=<rhtas-install-namespace>
    export TUF_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get tuf -o jsonpath='{.items[0].status.url}')"
    ```

2. Base64-Encode the root.json  
    Download the TUF root.json and encode it as Base64.
    ```sh
    export BASE64_TUF_ROOT="$(curl -fsSL "$TUF_URL/root.json" | base64 -w0)"
    ```

3. Create or Apply the TrustRoot CR (Cluster-Scoped)  
    Apply a TrustRoot Custom Resource using the mirror URL and encoded root.
    ```sh
    cat <<EOF | kubectl apply -f -
    apiVersion: policy.sigstore.dev/v1alpha1
    kind: TrustRoot
    metadata:
      name: trust-root
    spec:
      remote:
        mirror: $TUF_URL
        root: | 
          $BASE64_TUF_ROOT
    EOF
    ```

## Configuring TrustRoot for ‘bring your own keys’
1. Grab Fulcio, Rekor, CTLog and TSA URLs from your RHTAS install
    ```sh
    export RHTAS_INSTALL_NAMESPACE=<rhtas-install-namespace>
    export FULCIO_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get fulcio -o jsonpath='{.items[0].status.url}')"
    export CTLOG_URL="http://ctlog.$RHTAS_INSTALL_NAMESPACE.svc.cluster.local"
    export REKOR_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get rekor -o jsonpath='{.items[0].status.url}')"
    export TSA_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get timestampAuthorities -o jsonpath='{.items[0].status.url}')"
    ```

2. Retrieve your keys and certificates  
    Get the Fulcio certificate chain, TSA certificate chain (if applicable), CT log public key, and Rekor public key from the RHTAS instance you installed.
    ```sh
    export SECRET_DATA=$(oc -n "$RHTAS_INSTALL_NAMESPACE" get secret <secret-name> -o jsonpath='{.data.<key>}')
    ```

3. Base64-encode the secret data  
    Encode the retrieved secret data
    ```sh
    echo $SECRET_DATA | base64 -w0
    ```

4. Create or Apply the TrustRoot CR (Cluster-Scoped)  
    Use the template below to define your TrustRoot
    ```sh
    apiVersion: policy.sigstore.dev/v1alpha1
    kind: TrustRoot
    metadata:
      name: trust-root
    spec:
      sigstoreKeys:
        certificateAuthorities:
        - subject:
            organization: fulcio-organization
            commonName: fulcio-common-name
        uri: https://fulcio.fulcio-system.svc
        certChain: |-
            FULCIO_CERT_CHAIN
        ctLogs:
        - baseURL: https://ctfe.example.com
          hashAlgorithm: sha-256
          publicKey: |-
            CTFE_PUBLIC_KEY
        tLogs:
        - baseURL: https://rekor.rekor-system.svc
          hashAlgorithm: sha-256
          publicKey: |-
            REKOR_PUBLIC_KEY
        timestampAuthorities:
        - subject:
            organization: tsa-organization
            commonName: tsa-common-name
          uri: https://tsa.example.com
          certChain: |-
            TSA_CERT_CHAIN
    ```

## Configuring TrustRoot for Serialized Tuf Root
1. Retrieve and Encode the TUF Root  
    Get your TUF mirror URL from the RHTAS TUF resource, and Base64-encode the root.json.
    ```sh
    export RHTAS_INSTALL_NAMESPACE=<rhtas-install-namespace>
    export TUF_URL="$(oc -n "$RHTAS_INSTALL_NAMESPACE" get tuf -o jsonpath='{.items[0].status.url}')"
    export BASE64_TUF_ROOT="$(curl -fsSL "$TUF_URL/root.json" | base64 -w0)"
    ```

2. Prepare a Temporary Directory  
    Create a temporary directory where the TUF repository will be cloned.
    ```sh
    mkdir -p tuf-repo
    ```

3. Download root.json and Clone the TUF Repository  
    Use tuftool to clone the TUF metadata and targets into the temp directory.
    ```sh
    curl -s $TUF_URL/root.json > root.json
    tuftool clone --metadata-url=$TUF_URL --metadata-dir=tuf-repo --targets-url=$TUF_URL/targets --targets-dir=tuf-repo/targets --root=root.json
    ```

4. Package and Encode the TUF Repository  
    Tar the cloned repo and Base64-encode it for embedding in the CR.
    ```sh
    tar -C ./tuf-repo -czf tuf-repo.tgz .
    export MIRROR_FS=$(base64 -w0 tuf-repo.tgz)
    ```

5. Create or Apply the TrustRoot CR (Cluster-Scoped)  
    Apply a TrustRoot Custom Resource containing the serialized root and mirror filesystem.
    ```sh
    cat <<EOF | kubectl apply -f -
    apiVersion: policy.sigstore.dev/v1alpha1
    kind: TrustRoot
    metadata:
      name: trust-root
    spec:
      repository:
        root: |-
          $BASE64_TUF_ROOT
        mirrorFS: |-
          $MIRROR_FS
    EOF
    ```

For more configuration options please visit the upstream documentation: https://docs.sigstore.dev/policy-controller/overview/
