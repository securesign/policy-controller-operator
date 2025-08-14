# Configuring the Policy Controller resource
This document will guide you through configuring a basic Policy Controller resource for a Red Hat Trusted Artifact Signer (RHTAS) instance.

## Prerequisites
Before proceeding, ensure you have the following:
- A running RHTAS instance (Red Hat Trusted Artifact Signer)
- A running policy controller operator instance
- Required CLI tools installed:
    - oc

## Configuring a Basic Policy Controller Resource
The following example defines a basic Policy Controller instance that works on OpenShift once the Helm-based Policy Controller Operator is installed.
This configuration sets the Policy Controller to watch for resources in namespaces that match the label selector defined in spec.policy-controller.webhook.namespaceSelector.matchExpressions.
In this example, the controller will watch for any resource created in a namespace that has the label `policy.rhtas.com/include` set to 'true'.

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rhtas.charts.redhat.com/v1alpha1
kind: PolicyController
metadata:
  name: policycontroller-sample
spec:
  policy-controller:
    cosign:
      webhookName: "policy.rhtas.com"
    webhook:
      name: webhook
      extraArgs:
        webhook-name: policy.rhtas.com
        mutating-webhook-name: defaulting.clusterimagepolicy.rhtas.com
        validating-webhook-name: validating.clusterimagepolicy.rhtas.com
      failurePolicy: Fail
      namespaceSelector:
        matchExpressions:
          - key: policy.rhtas.com/include
            operator: In
            values: ["true"]
      webhookNames:
        defaulting: "defaulting.clusterimagepolicy.rhtas.com"
        validating: "validating.clusterimagepolicy.rhtas.com"
EOF
```

NOTE:
* The resource must be installed in the **policy-controller-operator** namespace.
* TUF is disabled by default (disable-tuf: true) to prevent the policy controller from trusting the Sigstore public good instance, which could allow untrusted resources to be deployed.
* When deploying an unreleased version of the policy controller, run `make dev-images` to update the image registry coordinates to quay.io before building.

## Sample Namespace
Below is an example namespace configuration that works with the policy controller definition shown above:

  ```sh
  cat <<EOF | kubectl apply -f -
  apiVersion: v1
  kind: Namespace
  metadata:
    labels:
      policy.rhtas.com/include: "true"
    name: policy-controller-test
  EOF
  ```

The `policy.rhtas.com/include: "true"` label marks this namespace for inclusion in policy controller operations. Any namespace with this label will be subject to the policies defined by cluster image policies.

## Custom Certificate Authority bundle
When deploying the Policy Controller operator, you may need to configure it to trust custom Certificate Authorities (CAs) or self-signed certificates. This is often necessary to ensure secure communication between components or with an external OIDC service.

Before configuring the Policy Controller operator to trust custom CAs, you must first create a ConfigMap containing your CA bundle in the same namespace where the Policy Controller will be deployed.

```sh
apiVersion: v1
kind: ConfigMap
metadata:
  name: custom-ca-bundle
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    MIIC... (certificate content)
    -----END CERTIFICATE-----
```

Once the ConfigMap is created, you can mount the CA bundle into the Policy Controller by setting the `spec.policy-controller.webhook.registryCaBundle` field.

```sh
apiVersion: rhtas.charts.redhat.com/v1alpha1
kind: PolicyController
metadata:
  name: policycontroller-sample
spec:
  policy-controller:
    webhook:
      registryCaBundle:
        name: <configMap-name>
        key: <ca-bundle-key>
```

You can also add environment variables, such as SSL_CERT_DIR, to the configuration:
```sh
apiVersion: rhtas.charts.redhat.com/v1alpha1
kind: PolicyController
metadata:
  name: policycontroller-sample
spec:
  policy-controller:
    webhook:
      env:
        SSL_CERT_DIR: <ssl-dir>
```

For more configuration options please visit the upstream helm charts: https://github.com/sigstore/helm-charts/tree/main/charts/policy-controller
