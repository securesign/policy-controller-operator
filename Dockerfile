## Build the admission-webhook-controller binary
FROM registry.redhat.io/ubi9/go-toolset:9.7@sha256:ca9697879a642fa691f17daf329fc24d239f5b385ecda06070ebddd5fdab287d AS admission-webhook-controller
WORKDIR /opt/app-root/src/
ENV GOEXPERIMENT=strictfipsruntime
ENV CGO_ENABLED=1

COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd

RUN go build -mod=mod -o admission-webhook-controller ./cmd

# Unpack Helm chart
FROM registry.redhat.io/ubi9/ubi@sha256:b8923f58ef6aebe2b8f543f8f6c5af15c6f9aeeef34ba332f33bf7610012de0c AS unpack-templates
WORKDIR /opt/app-root/src/
ENV HOME=/opt/app-root/src/

COPY helm-charts ${HOME}/helm-charts
RUN tar -xvf ${HOME}/helm-charts/policy-controller-operator/charts/policy-controller-*.tgz \
    -C ${HOME}/helm-charts/policy-controller-operator/charts/ && \
    rm ${HOME}/helm-charts/policy-controller-operator/charts/policy-controller-*.tgz

# Build the manager binary
FROM registry.redhat.io/openshift4/ose-helm-rhel9-operator@sha256:639ddf96bfc85f7c014a0917e56259fbf138a7e41cf399d49f8056f7f88616ad

LABEL description="The image for the policy-controller-operator."
LABEL io.k8s.description="The image for the policy-controller-operator."
LABEL io.k8s.display-name="Policy Controller operator container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="policy-controller-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator for the policy-controller-operator."
LABEL com.redhat.component="policy-controller-operator"
LABEL name="rhtas/policy-controller-rhel9-operator"
LABEL cpe="cpe:/a:redhat:trusted_artifact_signer:1.3.1::el9"

ENV HOME=/opt/helm
COPY watches.yaml ${HOME}/watches.yaml
COPY --from=unpack-templates /opt/app-root/src/helm-charts ${HOME}/helm-charts
COPY LICENSE /licenses/license.txt
COPY --from=admission-webhook-controller /opt/app-root/src/admission-webhook-controller /usr/local/bin/admission-webhook-controller

WORKDIR ${HOME}
