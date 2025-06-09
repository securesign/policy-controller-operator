package e2e_utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
)

const (
	defaultTufMirror = "http://tuf.local"
	tufMirrorEnv     = "TUF_URL"
)

func TufUrl() string {
	return EnvOrDefault(tufMirrorEnv, defaultTufMirror)
}

func ResolveBase64TufRoot(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/root.json", TufUrl())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	rootBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	encodedRoot := base64.StdEncoding.EncodeToString(rootBytes)
	return encodedRoot, nil
}
