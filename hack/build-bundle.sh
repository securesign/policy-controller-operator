#!/bin/bash
set -e

TOOLS="/tmp"
YQ_VERSION=v4.44.3
KUSTOMIZE_VERSION=v5.6.0

if [ -f "/cachi2/output/deps/generic/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz" ]
then
  tar -xzf /cachi2/output/deps/generic/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz -C ${TOOLS}
  KUSTOMIZE=${TOOLS}/kustomize
else
  curl -Lo ${TOOLS}/kustomize.tar.gz "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz" && \
  tar -xzf ${TOOLS}/kustomize.tar.gz -C ${TOOLS}
  rm ${TOOLS}/kustomize.tar.gz
  KUSTOMIZE=${TOOLS}/kustomize
fi
chmod +x ${KUSTOMIZE}

if [ -f "/cachi2/output/deps/generic/yq_linux_amd64.tar.gz" ]
then
  tar -xzf /cachi2/output/deps/generic/yq_linux_amd64.tar.gz -C "${TOOLS}"
  YQ=${TOOLS}/yq_linux_amd64
else
  curl -Lo ${TOOLS}/yq.tar.gz "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64.tar.gz" && \
  tar -xzf ${TOOLS}/yq.tar.gz -C ${TOOLS}
  rm ${TOOLS}/yq.tar.gz
  YQ=${TOOLS}/yq_linux_amd64
fi
chmod +x ${YQ}

if [[ -n "$IMG" ]]
then
  pushd config/manager
  ${KUSTOMIZE} edit set image controller="${IMG}"
  popd
fi

# Add related images
RELATED_IMAGE_POLICY_CONTROLLER_DIGEST="$("${YQ}" -r '.["policy-controller"].webhook.image.version' helm-charts/policy-controller-operator/values.yaml)"
RELATED_IMAGE_OSE_CLI_DIGEST="$("${YQ}" -r '.["policy-controller"].leasescleanup.image.version' helm-charts/policy-controller-operator/values.yaml)"
echo "RELATED_IMAGE_POLICY_CONTROLLER=registry.redhat.io/rhtas/policy-controller-rhel9@${RELATED_IMAGE_POLICY_CONTROLLER_DIGEST}" > config/manager/images.env
echo "RELATED_IMAGE_OSE_CLI=registry.redhat.io/openshift4/ose-cli@sha256:${RELATED_IMAGE_OSE_CLI_DIGEST}" >> config/manager/images.env

"${KUSTOMIZE}" build config/manifests | operator-sdk generate bundle ${BUNDLE_GEN_FLAGS:-}
operator-sdk bundle validate ./bundle
