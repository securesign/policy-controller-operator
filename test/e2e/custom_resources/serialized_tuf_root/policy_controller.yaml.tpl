apiVersion: rhtas.charts.redhat.com/v1alpha1
kind: PolicyController
metadata:
  name: policycontroller-sample
  namespace: policy-controller-operator
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
