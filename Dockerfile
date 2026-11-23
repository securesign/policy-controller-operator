# Build the admission-webhook-controller binary
FROM registry.redhat.io/ubi9/go-toolset:latest@sha256:23c7cb17e9c53a65f9ab01f2edf1a4e07c9a9dc2258ed63d69e06925e7cea381 AS admission-webhook-controller
WORKDIR /opt/app-root/src/
ENV CGO_ENABLED=0
ENV GOFIPS140=v1.0.0

COPY --chown=1001:0 go.mod go.mod
COPY --chown=1001:0 go.sum go.sum
COPY --chown=1001:0 cmd cmd

RUN go mod edit -godebug=fips140=auto && \
    go build -mod=mod -tags 'no_openssl' -o admission-webhook-controller ./cmd

# Unpack Helm chart
FROM registry.redhat.io/ubi9/ubi-minimal:latest@sha256:08d4c70f1baf6537a0728656cf7942d561e74a35916509d205b6519861266352 AS unpack-templates
WORKDIR /opt/app-root/src/
ENV HOME=/opt/app-root/src/

RUN microdnf install -y tar gzip && microdnf clean all
COPY helm-charts ${HOME}/helm-charts
RUN tar -xvf ${HOME}/helm-charts/policy-controller-operator/charts/policy-controller-*.tgz \
    -C ${HOME}/helm-charts/policy-controller-operator/charts/ && \
    rm ${HOME}/helm-charts/policy-controller-operator/charts/policy-controller-*.tgz

# Build the manager binary
FROM registry.redhat.io/openshift4/ose-helm-rhel9-operator:latest@sha256:1527eb20275f29f58f24d7d539a7aa35bb504815f661a49ff1348f18c618eabb

LABEL description="The image for the policy-controller-operator."
LABEL io.k8s.description="The image for the policy-controller-operator."
LABEL io.k8s.display-name="Policy Controller operator container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="policy-controller-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator for the policy-controller-operator."
LABEL com.redhat.component="policy-controller-operator"
LABEL name="rhtas/policy-controller-rhel9-operator"

ENV HOME=/opt/helm
COPY watches.yaml ${HOME}/watches.yaml
COPY --from=unpack-templates /opt/app-root/src/helm-charts ${HOME}/helm-charts
COPY LICENSE /licenses/license.txt
COPY --from=admission-webhook-controller /opt/app-root/src/admission-webhook-controller /usr/local/bin/admission-webhook-controller

WORKDIR ${HOME}
