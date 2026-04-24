# Build the admission-webhook-controller binary
FROM registry.redhat.io/ubi9/go-toolset:latest@sha256:75ecb48dfaec6655bd589091902e3b9328c300befba34cb06fc5e7323099e17d AS admission-webhook-controller
WORKDIR /opt/app-root/src/
ENV GOEXPERIMENT=strictfipsruntime
ENV CGO_ENABLED=1

COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd

RUN go build -mod=mod -o admission-webhook-controller ./cmd

# Unpack Helm chart
FROM registry.redhat.io/ubi9/ubi-minimal:latest@sha256:7d4e47500f28ac3a2bff06c25eff9127ff21048538ae03ce240d57cf756acd00 AS unpack-templates
WORKDIR /opt/app-root/src/
ENV HOME=/opt/app-root/src/

RUN microdnf install -y tar gzip && microdnf clean all
COPY helm-charts ${HOME}/helm-charts
RUN tar -xvf ${HOME}/helm-charts/policy-controller-operator/charts/policy-controller-*.tgz \
    -C ${HOME}/helm-charts/policy-controller-operator/charts/ && \
    rm ${HOME}/helm-charts/policy-controller-operator/charts/policy-controller-*.tgz

# Build the manager binary
FROM registry.redhat.io/openshift4/ose-helm-rhel9-operator:latest@sha256:b242a0d49a79c8f732612ed93b84c54077378b13b4ceb106e91864f5b02e2475

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
