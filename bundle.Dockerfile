FROM scratch

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=policy-controller-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.39.2
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=helm.sdk.operatorframework.io/v1

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

LABEL vendor="Red Hat, Inc."
LABEL url="https://www.redhat.com"
LABEL distribution-scope="public"
LABEL version="1.3.0"

LABEL description="The bundle image for the policy-controller-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.description="The bundle image for the policy-controller-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.display-name="Policy Controller operator bundle container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="policy-controller-operator-bundle, policy-controller-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator Bundle for the policy-controller-operator."
LABEL com.redhat.component="policy-controller-operator-bundle"
LABEL name="policy-controller-operator-bundle"

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/
