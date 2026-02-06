apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name:  {{ .NAME }}
  namespace: {{ .NS }}
spec:
  channel: {{ .CHANNEL }}
  installPlanApproval: Automatic
  name: policy-controller-operator
  source: {{ .SOURCE }}
  sourceNamespace: {{ .SOURCE_NAMESPACE }}
