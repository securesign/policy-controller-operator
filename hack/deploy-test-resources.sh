#!/usr/bin/env bash
set -euo pipefail

###############################################################################
# Deploy PolicyController + TrustRoot + ClusterImagePolicy for manual testing.
#
# Prerequisites:
#   - oc / kubectl logged into the target cluster
#   - RHTAS operator installed (Fulcio, Rekor, TUF, etc. running)
#   - policy-controller-operator installed (CRDs present)
#
# Usage:
#   ./hack/deploy-test-resources.sh [flags]
#
# Flags:
#   --image <ref>             Override the policy-controller webhook image
#                             (e.g. quay.io/user/policy-controller@sha256:...)
#   --rhtas-ns <namespace>    RHTAS install namespace (default: trusted-artifact-signer)
#   --keycloak-ns <namespace> Keycloak namespace (default: keycloak-system)
#   --pc-ns <namespace>       PolicyController namespace (default: policy-controller-operator)
#   --test-ns <namespace>     Test namespace to label for enforcement (default: test-policy)
#   --trust-root <name>       TrustRoot resource name (default: test-trust-root)
#   --cip <name>              ClusterImagePolicy name (default: test-cip)
#   --image-glob <glob>       Image glob for CIP (default: **)
#   --bundle-format           Set signatureFormat: bundle on the CIP authority
#                             (for testing cosign v3 bundle-format signatures)
#   --cleanup                 Delete all test resources and exit
#   --dry-run                 Print manifests without applying
###############################################################################

RHTAS_NS="${RHTAS_NS:-trusted-artifact-signer}"
KEYCLOAK_NS="${KEYCLOAK_NS:-keycloak-system}"
PC_NS="${PC_NS:-policy-controller-operator}"
TEST_NS="${TEST_NS:-test-policy}"
TRUST_ROOT_NAME="${TRUST_ROOT_NAME:-test-trust-root}"
CIP_NAME="${CIP_NAME:-test-cip}"
IMAGE_GLOB="${IMAGE_GLOB:-**}"
WEBHOOK_IMAGE=""
BUNDLE_FORMAT=false
CLEANUP=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)        WEBHOOK_IMAGE="$2"; shift 2 ;;
    --rhtas-ns)     RHTAS_NS="$2"; shift 2 ;;
    --keycloak-ns)  KEYCLOAK_NS="$2"; shift 2 ;;
    --pc-ns)        PC_NS="$2"; shift 2 ;;
    --test-ns)      TEST_NS="$2"; shift 2 ;;
    --trust-root)   TRUST_ROOT_NAME="$2"; shift 2 ;;
    --cip)          CIP_NAME="$2"; shift 2 ;;
    --image-glob)   IMAGE_GLOB="$2"; shift 2 ;;
    --bundle-format) BUNDLE_FORMAT=true; shift ;;
    --cleanup)      CLEANUP=true; shift ;;
    --dry-run)      DRY_RUN=true; shift ;;
    -h|--help)
      sed -n '5,/^###/{ /^###/d; s/^# \{0,1\}//; p; }' "$0"
      exit 0
      ;;
    *) echo "Unknown flag: $1" >&2; exit 1 ;;
  esac
done

info()  { echo "===> $*"; }
apply() {
  if $DRY_RUN; then
    echo "---"
    echo "$1"
  else
    echo "$1" | oc apply -f -
  fi
}

# --- Cleanup mode -----------------------------------------------------------
if $CLEANUP; then
  info "Cleaning up test resources"
  oc delete clusterimagepolicy "$CIP_NAME" --ignore-not-found 2>/dev/null || true
  oc delete trustroot "$TRUST_ROOT_NAME" --ignore-not-found 2>/dev/null || true
  oc delete policycontroller policycontroller-sample -n "$PC_NS" --ignore-not-found 2>/dev/null || true
  oc delete namespace "$TEST_NS" --ignore-not-found 2>/dev/null || true
  info "Cleanup complete"
  exit 0
fi

# --- Discover RHTAS service URLs from OpenShift routes -----------------------
info "Discovering RHTAS URLs from namespace: $RHTAS_NS"

get_route_url() {
  local prefix="$1"
  local namespace="$2"
  local route_json
  # Route names may have random suffixes (e.g. fulcio-server-t85t5), so match by prefix
  route_json=$(oc get routes -n "$namespace" -o json 2>/dev/null \
    | jq -r --arg p "$prefix" '.items[] | select(.metadata.name | startswith($p)) | {host: .spec.host, tls: .spec.tls} | @json' \
    | head -1) || true
  if [[ -z "$route_json" ]]; then
    echo "ERROR: No route matching '${prefix}*' found in namespace '$namespace'" >&2
    return 1
  fi
  local host tls scheme
  host=$(echo "$route_json" | jq -r '.host')
  tls=$(echo "$route_json" | jq -r '.tls')
  scheme="https"
  if [[ "$tls" == "null" || -z "$tls" ]]; then
    scheme="http"
  fi
  echo "${scheme}://${host}"
}

FULCIO_URL="${COSIGN_FULCIO_URL:-$(get_route_url fulcio-server "$RHTAS_NS")}"
REKOR_URL="${COSIGN_REKOR_URL:-$(get_route_url rekor-server "$RHTAS_NS")}"
TUF_URL="${TUF_URL:-$(get_route_url tuf "$RHTAS_NS")}"

OIDC_ISSUER_URL="${OIDC_ISSUER_URL:-}"
if [[ -z "$OIDC_ISSUER_URL" ]]; then
  KEYCLOAK_HOST=$(oc get routes -n "$KEYCLOAK_NS" -o json 2>/dev/null \
    | jq -r '.items[] | select(.metadata.name | startswith("keycloak")) | .spec.host' \
    | head -1) || true
  if [[ -n "$KEYCLOAK_HOST" ]]; then
    # Newer Keycloak uses /realms/, older uses /auth/realms/ — probe to find which
    if curl -sSf "https://${KEYCLOAK_HOST}/realms/trusted-artifact-signer/.well-known/openid-configuration" &>/dev/null; then
      OIDC_ISSUER_URL="https://${KEYCLOAK_HOST}/realms/trusted-artifact-signer"
    else
      OIDC_ISSUER_URL="https://${KEYCLOAK_HOST}/auth/realms/trusted-artifact-signer"
    fi
  else
    OIDC_ISSUER_URL="http://keycloak-internal.${KEYCLOAK_NS}.svc/auth/realms/trusted-artifact-signer"
  fi
fi
OIDC_ISSUER_SUBJECT="${OIDC_ISSUER_SUBJECT:-jdoe@redhat.com}"

info "  FULCIO_URL:       $FULCIO_URL"
info "  REKOR_URL:        $REKOR_URL"
info "  TUF_URL:          $TUF_URL"
info "  OIDC_ISSUER_URL:  $OIDC_ISSUER_URL"
info "  KEYCLOAK_NS:      $KEYCLOAK_NS"

# --- Fetch TUF root.json ----------------------------------------------------
info "Fetching TUF root.json from $TUF_URL/root.json"
TUF_ROOT_RAW=$(curl -sSfL "${TUF_URL}/root.json")
TUF_ROOT_B64=$(echo -n "$TUF_ROOT_RAW" | base64 -w0)

# --- Build PolicyController manifest ----------------------------------------
info "Creating PolicyController in namespace: $PC_NS"

IMAGE_BLOCK=""
if [[ -n "$WEBHOOK_IMAGE" ]]; then
  if [[ "$WEBHOOK_IMAGE" == *"@sha256:"* ]]; then
    REPO="${WEBHOOK_IMAGE%%@sha256:*}"
    DIGEST="${WEBHOOK_IMAGE#*@sha256:}"
    IMAGE_BLOCK="      image:
        repository: ${REPO}
        version: sha256:${DIGEST}
        pullPolicy: IfNotPresent"
  elif [[ "$WEBHOOK_IMAGE" == *":"* ]]; then
    REPO="${WEBHOOK_IMAGE%%:*}"
    TAG="${WEBHOOK_IMAGE#*:}"
    IMAGE_BLOCK="      image:
        repository: ${REPO}
        version: ${TAG}
        pullPolicy: Always"
  else
    IMAGE_BLOCK="      image:
        repository: ${WEBHOOK_IMAGE}
        version: latest
        pullPolicy: Always"
  fi
  info "  Using custom image: $WEBHOOK_IMAGE"
fi

PC_MANIFEST="apiVersion: rhtas.charts.redhat.com/v1alpha1
kind: PolicyController
metadata:
  name: policycontroller-sample
  namespace: ${PC_NS}
spec:
  policy-controller:
    cosign:
      webhookName: \"policy.rhtas.com\"
    webhook:
      name: webhook
${IMAGE_BLOCK:+${IMAGE_BLOCK}
}      extraArgs:
        webhook-name: policy.rhtas.com
        mutating-webhook-name: defaulting.clusterimagepolicy.rhtas.com
        validating-webhook-name: validating.clusterimagepolicy.rhtas.com
        disable-tuf: true
      failurePolicy: Fail
      namespaceSelector:
        matchExpressions:
          - key: policy.rhtas.com/include
            operator: In
            values: [\"true\"]
      webhookNames:
        defaulting: \"defaulting.clusterimagepolicy.rhtas.com\"
        validating: \"validating.clusterimagepolicy.rhtas.com\""

apply "$PC_MANIFEST"

# --- Wait for webhook deployment to be ready ---------------------------------
if ! $DRY_RUN; then
  DEPLOYMENT_NAME="policycontroller-sample-policy-controller-webhook"
  info "Waiting for deployment $DEPLOYMENT_NAME to exist..."
  for i in $(seq 1 60); do
    if oc get deployment "$DEPLOYMENT_NAME" -n "$PC_NS" &>/dev/null; then
      break
    fi
    if [[ $i -eq 60 ]]; then
      echo "ERROR: Deployment $DEPLOYMENT_NAME was not created after 120s" >&2
      exit 1
    fi
    sleep 2
  done
  info "Waiting for deployment $DEPLOYMENT_NAME to be ready..."
  oc rollout status deployment/"$DEPLOYMENT_NAME" -n "$PC_NS" --timeout=120s
fi

# --- Create TrustRoot -------------------------------------------------------
info "Creating TrustRoot: $TRUST_ROOT_NAME"

TR_MANIFEST="apiVersion: policy.sigstore.dev/v1alpha1
kind: TrustRoot
metadata:
  name: ${TRUST_ROOT_NAME}
spec:
  remote:
    mirror: ${TUF_URL}
    root: |
      ${TUF_ROOT_B64}"

apply "$TR_MANIFEST"

# --- Wait for TrustRoot to be reconciled into ConfigMap ----------------------
if ! $DRY_RUN; then
  info "Waiting for TrustRoot to be reconciled into config-sigstore-keys ConfigMap..."
  for i in $(seq 1 30); do
    if oc get configmap config-sigstore-keys -n "$PC_NS" -o jsonpath="{.data.${TRUST_ROOT_NAME}}" 2>/dev/null | grep -q .; then
      info "  TrustRoot reconciled successfully"
      break
    fi
    if [[ $i -eq 30 ]]; then
      echo "ERROR: TrustRoot was not reconciled after 60s. Check 'oc get configmap config-sigstore-keys -n $PC_NS'" >&2
      exit 1
    fi
    sleep 2
  done
fi

# --- Create ClusterImagePolicy ----------------------------------------------
info "Creating ClusterImagePolicy: $CIP_NAME"

SIGNATURE_FORMAT_BLOCK=""
RFC3161_BLOCK=""
if $BUNDLE_FORMAT; then
  info "  Using signatureFormat: bundle (cosign v3)"
  SIGNATURE_FORMAT_BLOCK="      signatureFormat: bundle"
  RFC3161_BLOCK="      rfc3161timestamp:
        trustRootRef: ${TRUST_ROOT_NAME}"
fi

CIP_MANIFEST="apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: ${CIP_NAME}
spec:
  images:
    - glob: \"${IMAGE_GLOB}\"
  authorities:
    - keyless:
        url: ${FULCIO_URL}
        trustRootRef: ${TRUST_ROOT_NAME}
        identities:
          - issuerRegExp: .*
            subjectRegExp: .*
      ctlog:
        url: ${REKOR_URL}
        trustRootRef: ${TRUST_ROOT_NAME}
${RFC3161_BLOCK:+${RFC3161_BLOCK}
}${SIGNATURE_FORMAT_BLOCK:+${SIGNATURE_FORMAT_BLOCK}}"

apply "$CIP_MANIFEST"

# --- Wait for CIP to be reconciled ------------------------------------------
if ! $DRY_RUN; then
  info "Waiting for CIP to be reconciled into config-image-policies ConfigMap..."
  for i in $(seq 1 30); do
    if oc get configmap config-image-policies -n "$PC_NS" -o jsonpath="{.data.${CIP_NAME}}" 2>/dev/null | grep -q .; then
      info "  CIP reconciled successfully"
      break
    fi
    if [[ $i -eq 30 ]]; then
      echo "ERROR: CIP was not reconciled after 60s. Check 'oc get configmap config-image-policies -n $PC_NS'" >&2
      exit 1
    fi
    sleep 2
  done
fi

# --- Create and label test namespace -----------------------------------------
info "Creating test namespace: $TEST_NS (labeled for policy enforcement)"
if ! $DRY_RUN; then
  oc create namespace "$TEST_NS" --dry-run=client -o yaml | oc apply -f -
  oc label namespace "$TEST_NS" policy.rhtas.com/include=true --overwrite
fi

# --- Summary -----------------------------------------------------------------
echo ""
info "Deployment complete!"
echo ""
echo "Resources created:"
echo "  PolicyController:    policycontroller-sample  (ns: $PC_NS)"
echo "  TrustRoot:           $TRUST_ROOT_NAME         (cluster-scoped)"
echo "  ClusterImagePolicy:  $CIP_NAME                (cluster-scoped)"
echo "  Test namespace:      $TEST_NS                 (labeled for enforcement)"
echo ""
echo "Cleanup:"
echo "  $0 --cleanup"
