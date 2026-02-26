apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: {{ .NAME }}
  namespace: {{ .NS }}
spec:
  targetNamespaces: []
