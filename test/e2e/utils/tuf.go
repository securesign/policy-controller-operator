package e2e_utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultExternalTufMirror = "http://tuf.local"
	defaultInternalTufMirror = "http://tuf.openshift-rhtas-operator.svc.cluster.local"

	tufExternalMirrorEnv = "TUF_EXTERNAL_MIRROR"
	tufInternalMirrorEnv = "TUF_INTERNAL_MIRROR"
)

func TufExternalUrl() string {
	return EnvOrDefault(tufExternalMirrorEnv, defaultExternalTufMirror)
}

func TufInternalUrl() string {
	return EnvOrDefault(tufInternalMirrorEnv, defaultInternalTufMirror)
}

func TufUrl(ci bool) string {
	if ci {
		return TufInternalUrl()
	}
	return TufExternalUrl()
}

func ResolveBase64TufRoot(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/root.json", TufExternalUrl())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}

	rootBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(rootBytes), nil
}
