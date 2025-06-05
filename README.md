# Policy Controller Operator
A Helm‑based operator for deploying and managing instances of the Sigstore Policy Controller on OpenShift/Kubernetes.

## Getting Started
You’ll need access to a Kubernetes cluster (either OpenShift or Kubernetes) to install and run this operator.

### Running on the cluster
1. Build and push the operator image:
```sh
IMG=<registry>/operator:tag make docker-build docker-push
```

2. Deploy the operator with the image you just pushed:
```sh
IMG=<registry>/operator:tag make deploy
```

3. Create the PolicyController custom resource:

Modify the sample manifest at config/samples/rhtas.charts_v1alpha1_policycontroller.yaml, then apply it:
```sh
kubectl apply -f config/samples/rhtas.charts_v1alpha1_policycontroller.yaml
```

NOTE:
* The resource must be installed in the **policy-controller-operator** namespace.
* TUF is disabled by default (disable-tuf: true) to prevent the policy controller from trusting the Sigstore public good instance, which could allow untrusted resources to be deployed.

The Policy Controller should now be deployed to your cluster

## Create a Trust Root
When using this operator, it is assumed you are using **RHTAS** (Red Hat Trusted Artifact Signer) as your Sigstore instance. In order to allow the policy controller to trust your instance, you need to create a **TrustRoot**. You will need two things to do this:

1. **TUF mirror URL**  
2. **Base64-encoded `root.json`**

You can then create a simple TrustRoot like so:

```yaml
apiVersion: policy.sigstore.dev/v1alpha1
kind: TrustRoot
metadata:
  name: trust-root
spec:
  remote:
    mirror: https://tuf.example.com
    root: |
      <BASE64_ENCODED_ROOT_JSON>
```

* Replace https://tuf.example.com with your actual TUF mirror URL.
* Replace <BASE64_ENCODED_ROOT_JSON> with the Base64-encoded contents of your root.json.

## Create a Cluster Image Policy
In order to enforce policy on resources, you need to create a **ClusterImagePolicy**. A simple policy might look like the example below:

```yaml
apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: cluster-image-policy
spec:
  images:
    - glob: "**"
  authorities:
    - keyless:
        url: https://fulcio.example.com
        trustRootRef: trust-root-ref
        identities:
          - issuer: https://oidc.example.com
            subject: oidc-issuer-subject
      ctlog:
        url: https://rekor.example.com
        trustRootRef: trust-root-ref
```

Replace the example URLs and trustRootRef values with your actual Fulcio, Rekor, and TrustRoot names. Once applied, this policy will only allow images that have been signed via your Sigstore setup (using Fulcio + Rekor) and match the specified OIDC identity.

By Default, the policy controller will enforce cluster image policies on namespaces that have the label `policy.rhtas.com/include=true`.

## Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
Undeploy the controller from the cluster:

```sh
make undeploy
```

# Documentation
For more information on the Policy controller please visit the upstream documentation: https://docs.sigstore.dev/policy-controller/overview/
