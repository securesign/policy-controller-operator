# Build the admission-webhook-controller binary
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.23@sha256:96cfceb50f5323efa1aa8569d4420cdbf1bb391225d5171ef72a0d0ecf028467 AS admission-webhook-controller
WORKDIR /opt/app-root/src/

COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd

RUN go build -mod=mod -o admission-webhook-controller ./cmd

# Build the manager binary
FROM registry.redhat.io/openshift4/ose-helm-rhel9-operator@sha256:b25e2dc071ff9e7f7bfd68ae3cd652ae2d3a4e5935f6dfaf880362b40cf372dc

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
