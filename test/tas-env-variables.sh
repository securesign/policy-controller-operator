#!/bin/bash

export RHTAS_INSTALL_NAMESPACE="${RHTAS_INSTALL_NAMESPACE:-openshift-rhtas-operator}"
if [ -z "$OIDC_ISSUER_URL" ]; then
  export OIDC_ISSUER_URL=https://$(oc get route keycloak -n keycloak-system | tail -n 1 | awk '{print $2}')/auth/realms/trusted-artifact-signer
fi

if [ -z "$TUF_URL" ]; then
  export TUF_URL=$(oc get tuf -o jsonpath='{.items[0].status.url}' -n ${RHTAS_INSTALL_NAMESPACE})
fi

if [ -z "$FULCIO_URL" ]; then
  export COSIGN_FULCIO_URL=$(oc get fulcio -o jsonpath='{.items[0].status.url}' -n ${RHTAS_INSTALL_NAMESPACE})
  else
    export COSIGN_FULCIO_URL=$FULCIO_URL
fi

if [ -z "$REKOR_URL" ]; then
  export COSIGN_REKOR_URL=$(oc get rekor -o jsonpath='{.items[0].status.url}' -n ${RHTAS_INSTALL_NAMESPACE})
  else
    export COSIGN_REKOR_URL=$REKOR_URL
fi

if [ -z "$REKOR_UI_URL" ]; then
  export REKOR_UI_URL=$(oc get rekor -o jsonpath='{.items[0].status.rekorSearchUIUrl}' -n ${RHTAS_INSTALL_NAMESPACE})
fi

if [ -z "$TSA_URL" ]; then
  export TSA_URL=$(oc get timestampauthorities -o jsonpath='{.items[0].status.url}' -n ${RHTAS_INSTALL_NAMESPACE})/api/v1/timestamp
fi

if [ -z "$OIDC_CLIENT_ID" ]; then
  OIDC_CLIENT_ID="trusted-artifact-signer"
fi

# Export the environment variables for the current session
export COSIGN_MIRROR=$TUF_URL
export COSIGN_ROOT=$TUF_URL/root.json
export COSIGN_OIDC_CLIENT_ID=$OIDC_CLIENT_ID
export COSIGN_OIDC_ISSUER=$OIDC_ISSUER_URL
export COSIGN_CERTIFICATE_OIDC_ISSUER=$OIDC_ISSUER_URL
export COSIGN_YES="true"
export SIGSTORE_FULCIO_URL=$COSIGN_FULCIO_URL
export SIGSTORE_OIDC_ISSUER=$COSIGN_OIDC_ISSUER
export SIGSTORE_REKOR_URL=$COSIGN_REKOR_URL
export REKOR_REKOR_SERVER=$COSIGN_REKOR_URL
export SIGSTORE_OIDC_CLIENT_ID=$OIDC_CLIENT_ID
export SIGSTORE_REKOR_UI_URL=$REKOR_UI_URL

# Print the environment variables to verify they are set
echo "TUF_URL=$TUF_URL"
echo "OIDC_ISSUER_URL=$OIDC_ISSUER_URL"
echo "COSIGN_FULCIO_URL=$COSIGN_FULCIO_URL"
echo "COSIGN_REKOR_URL=$COSIGN_REKOR_URL"
echo "COSIGN_MIRROR=$COSIGN_MIRROR"
echo "COSIGN_ROOT=$COSIGN_ROOT"
echo "COSIGN_OIDC_CLIENT_ID=$COSIGN_OIDC_CLIENT_ID"
echo "COSIGN_OIDC_ISSUER=$COSIGN_OIDC_ISSUER"
echo "COSIGN_CERTIFICATE_OIDC_ISSUER=$COSIGN_CERTIFICATE_OIDC_ISSUER"
echo "COSIGN_YES=$COSIGN_YES"
echo "SIGSTORE_FULCIO_URL=$SIGSTORE_FULCIO_URL"
echo "SIGSTORE_OIDC_ISSUER=$SIGSTORE_OIDC_ISSUER"
echo "SIGSTORE_REKOR_URL=$SIGSTORE_REKOR_URL"
echo "REKOR_REKOR_SERVER=$REKOR_REKOR_SERVER"
echo "SIGSTORE_OIDC_CLIENT_ID=$SIGSTORE_OIDC_CLIENT_ID"
echo "TSA_URL=$TSA_URL"
echo "SIGSTORE_REKOR_UI_URL=$SIGSTORE_REKOR_UI_URL"
echo "REKOR_UI_URL=$REKOR_UI_URL"
