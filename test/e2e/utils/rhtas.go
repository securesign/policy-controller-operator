package e2e_utils

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/sigstore/sigstore-go/pkg/tuf"
)

const (
	defaultFulcioUrl = "http://fulcio-server.local"
	fulcioEnv        = "COSIGN_FULCIO_URL"

	defaultRekorUrl = "http://rekor-server.local"
	rekorEnv        = "COSIGN_REKOR_URL"

	defaultTsaUrl = "http://tsa-server.local"
	tsaEnv        = "TSA_URL"

	defaultTufMirror = "http://tuf.local"
	tufMirrorEnv     = "TUF_URL"

	defaultRhtasInstallNamespace = "openshift-rhtas-operator"
	rhtasInstallNamespaceEnv     = "RHTAS_INSTALL_NAMESPACE"

	fulcioTufTarget = "fulcio_v1.crt.pem"
	tsaTufTarget    = "tsa.certchain.pem"
	ctlogTufTarget  = "ctfe.pub"
	rekorTufTarget  = "rekor.pub"
)

type TrustRootValues struct {
	FulcioOrgName    string
	FulcioCommonName string
	FulcioCertChain  string
	CtfePublicKey    string
	CtLogHashAlgo    string
	RekorPublicKey   string
	RekorHashAlgo    string
	TsaOrgName       string
	TsaCommonName    string
	TsaCertChain     string
}

func FulcioUrl() string {
	return EnvOrDefault(fulcioEnv, defaultFulcioUrl)
}

func RekorUrl() string {
	return EnvOrDefault(rekorEnv, defaultRekorUrl)
}

func TsaUrl() string {
	return EnvOrDefault(tsaEnv, defaultTsaUrl)
}

func TufUrl() string {
	return EnvOrDefault(tufMirrorEnv, defaultTufMirror)
}

func RhtasInstallNamespace() string {
	return EnvOrDefault(rhtasInstallNamespaceEnv, defaultRhtasInstallNamespace)
}

func ParseTufRoot(ctx context.Context) (*TrustRootValues, error) {
	tufRoot, err := ResolveTufRoot(ctx)
	if err != nil {
		return nil, err
	}
	opts := tuf.Options{
		Root:              tufRoot,
		RepositoryBaseURL: TufUrl(),
		DisableLocalCache: true,
	}
	client, err := tuf.New(&opts)
	if err != nil {
		return nil, fmt.Errorf("init TUF client: %w", err)
	}

	raw := make(map[string][]byte, 4)
	for _, name := range []string{fulcioTufTarget, tsaTufTarget, ctlogTufTarget, rekorTufTarget} {
		b, err := client.GetTarget(name)
		if err != nil {
			return nil, err
		}
		raw[name] = b
	}

	fCN, fOrg, fChain, err := parsePEMBundle(raw[fulcioTufTarget])
	if err != nil {
		return nil, fmt.Errorf("parse fulcio cert: %w", err)
	}

	tCN, tOrg, tChain, err := parsePEMBundle(raw[tsaTufTarget])
	if err != nil {
		return nil, fmt.Errorf("parse tsa cert: %w", err)
	}

	ctKey, ctAlgo, err := parsePubKey(raw[ctlogTufTarget])
	if err != nil {
		return nil, fmt.Errorf("parse ctfe key: %w", err)
	}

	rKey, rAlgo, err := parsePubKey(raw[rekorTufTarget])
	if err != nil {
		return nil, fmt.Errorf("parse rekor key: %w", err)
	}

	return &TrustRootValues{
		FulcioOrgName:    fOrg,
		FulcioCommonName: fCN,
		FulcioCertChain:  Base64EncodeString([]byte(fChain)),
		CtfePublicKey:    ctKey,
		CtLogHashAlgo:    ctAlgo,
		RekorPublicKey:   rKey,
		RekorHashAlgo:    rAlgo,
		TsaOrgName:       tOrg,
		TsaCommonName:    tCN,
		TsaCertChain:     Base64EncodeString([]byte(tChain)),
	}, nil
}

func ResolveTufRoot(ctx context.Context) ([]byte, error) {
	url := fmt.Sprintf("%s/root.json", TufUrl())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return []byte{}, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()

	rootBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	return rootBytes, nil
}

func parsePEMBundle(pemBytes []byte) (cn, org, bundle string, err error) {
	var block *pem.Block
	rest := pemBytes
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			return "", "", "", errors.New("no cert in bundle")
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return "", "", "", err
			}
			cn = cert.Subject.CommonName
			if len(cert.Subject.Organization) > 0 {
				org = cert.Subject.Organization[0]
			}
			break
		}
	}
	return cn, org, string(pemBytes), nil
}

func parsePubKey(pemBytes []byte) (keyB64, algo string, err error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return "", "", errors.New("no key found")
	}

	keyB64 = Base64EncodeString(pemBytes)
	switch block.Type {
	case "PUBLIC KEY":
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return "", "", err
		}
		switch pub.(type) {
		case *ecdsa.PublicKey:
			algo = "sha256"
		default:
			algo = "unknown"
		}
	default:
		algo = "unknown"
	}

	return keyB64, algo, nil
}
