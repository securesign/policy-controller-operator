# Build the admission-webhook-controller binary
FROM registry.redhat.io/ubi9/go-toolset:latest@sha256:4053e919374d739ee6b0007e412d89256b0bd065e0bc690c2b3905bbaab6500e AS admission-webhook-controller
WORKDIR /opt/app-root/src/
ENV GOEXPERIMENT=strictfipsruntime
ENV CGO_ENABLED=1

COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd

RUN go build -mod=mod -o admission-webhook-controller ./cmd

# Unpack Helm chart
FROM registry.redhat.io/ubi9/ubi-minimal:latest@sha256:baafce2d34783a28b4a472779f304e97e00f9d35c99553bc53b068b14344d9a9 AS unpack-templates
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
