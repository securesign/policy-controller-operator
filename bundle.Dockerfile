ARG VERSION="0.0.1"
ARG CHANNELS="tech-preview"
ARG DEFAULT_CHANNEL="tech-preview"
ARG BUNDLE_OVERLAY="olm"
ARG BUNDLE_GEN_FLAGS="-q --overwrite=false --version $VERSION --channels=$CHANNELS --default-channel=$DEFAULT_CHANNEL"
ARG IMG

FROM registry.redhat.io/openshift4/ose-operator-sdk-rhel9@sha256:ff74412d980d64ad54061589e74b68675276adcd69717f5b5befbe6310c0fc62 AS builder

ARG BUNDLE_GEN_FLAGS
ARG IMG

WORKDIR /tmp

COPY ./config/ ./config/
COPY PROJECT .
COPY hack/build-bundle.sh build-bundle.sh
COPY helm-charts helm-charts

USER root

RUN ./build-bundle.sh

FROM scratch

ARG CHANNELS
ARG VERSION

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=policy-controller-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.39.2
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=helm.sdk.operatorframework.io/v1

LABEL maintainer="Red Hat, Inc."
LABEL vendor="Red Hat, Inc."
LABEL url="https://www.redhat.com"
LABEL distribution-scope="public"
LABEL version=$VERSION

LABEL description="The bundle image for the Policy Controller Operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.description="The bundle image for the Policy Controller Operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.display-name="RHTAS Policy Controller Operator bundle container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="policy-controller-operator-bundle, policy-controller-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator Bundle for the Policy Controller Operator."
LABEL com.redhat.component="policy-controller-operator-bundle"
LABEL name="rhtas/policy-controller-operator-bundle"

LABEL features.operators.openshift.io/cni="false"
LABEL features.operators.openshift.io/disconnected="true"
LABEL features.operators.openshift.io/fips-compliant="false"
LABEL features.operators.openshift.io/proxy-aware="false"
LABEL features.operators.openshift.io/cnf="false"
LABEL features.operators.openshift.io/csi="false"
LABEL features.operators.openshift.io/tls-profiles="false"
LABEL features.operators.openshift.io/token-auth-aws="false"
LABEL features.operators.openshift.io/token-auth-azure="false"
LABEL features.operators.openshift.io/token-auth-gcp="false"

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY --from=builder /tmp/bundle/manifests /manifests/
COPY --from=builder /tmp/bundle/metadata /metadata/
COPY --from=builder /tmp/bundle/tests/scorecard /tests/scorecard/
COPY LICENSE /licenses/license.txt
