apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: {{ .NAME }}
  namespace: {{ .NS }}
spec:
  sourceType: grpc
  image: {{ .INDEX_IMAGE }}
  displayName: PCO Upgrade Test Catalog
  publisher: policy-controller-operator-e2e
