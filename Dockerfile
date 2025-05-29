# Build the admission-webhook-controller binary
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.23@sha256:4805e1cb2d1bd9d3c5de5d6986056bbda94ca7b01642f721d83d26579d333c60 AS admission-webhook-controller
WORKDIR /opt/app-root/src/
USER root

COPY go.mod go.mod
COPY cmd cmd

RUN go build -mod=mod -o admission-webhook-controller ./cmd

# Build the manager binary
FROM registry.redhat.io/openshift4/ose-helm-rhel9-operator@sha256:23c23ad0d341a91091da169951ecd475418e7798eb32917c6f032685c3e61f3b

LABEL description="The image for the policy-controller-operator."
LABEL io.k8s.description="The image for the policy-controller-operator."
LABEL io.k8s.display-name="Policy Controller operator container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="policy-controller-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator for the policy-controller-operator."
LABEL com.redhat.component="policy-controller-operator"
LABEL name="policy-controller-operator"

ENV HOME=/opt/helm
COPY watches.yaml ${HOME}/watches.yaml
COPY helm-charts  ${HOME}/helm-charts
COPY LICENSE /licenses/license.txt
COPY --from=admission-webhook-controller /opt/app-root/src/admission-webhook-controller /usr/local/bin/admission-webhook-controller

WORKDIR ${HOME}
