package e2e_utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
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

	fulcioTufTarget      = "fulcio_v1.crt.pem"
	tsaTufTarget         = "tsa.certchain.pem"
	ctlogTufTarget       = "ctfe.pub"
	rekorTufTarget       = "rekor.pub"
	trustedRootTufTarget = "trusted_root.json"
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

func TufMirrorFS(ctx context.Context) ([]byte, error) {
	tufRoot, err := ResolveTufRoot(ctx)
	if err != nil {
		return nil, err
	}

	tufRootDir, err := os.MkdirTemp(".", "tuf-repo-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tufRootDir)

	cfg, err := config.New(TufUrl(), tufRoot)
	if err != nil {
		return nil, err
	}

	cfg.RemoteTargetsURL = TufUrl() + "/targets"
	cfg.RemoteMetadataURL = TufUrl()
	cfg.LocalMetadataDir = tufRootDir
	cfg.LocalTargetsDir = filepath.Join(tufRootDir, "targets")
	if err := cfg.EnsurePathsExist(); err != nil {
		return nil, err
	}

	cfg.PrefixTargetsWithHash = true
	up, err := updater.New(cfg)
	if err != nil {
		return nil, err
	}

	for _, name := range []string{fulcioTufTarget, tsaTufTarget, ctlogTufTarget, rekorTufTarget, trustedRootTufTarget} {
		info, err := up.GetTargetInfo(name)
		if err != nil {
			return nil, err
		}
		var hashPrefix string
		for _, v := range info.Hashes {
			hashPrefix = hex.EncodeToString(v)
			break
		}
		hashedFileName := fmt.Sprintf("%s.%s", hashPrefix, filepath.Base(name))
		localPath := filepath.Join(tufRootDir, "targets", hashedFileName)
		if _, _, err := up.DownloadTarget(info, localPath, ""); err != nil {
			return nil, err
		}

	}

	for _, role := range []string{"snapshot", "root", "targets"} {
		unversioned := filepath.Join(cfg.LocalMetadataDir, role+".json")

		if err := writeVersionedMetadataFile(unversioned, cfg.LocalMetadataDir); err != nil {
			return nil, err
		}
		if err := os.Remove(unversioned); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if err := TarGZ(tufRootDir, &buf); err != nil {
		return nil, fmt.Errorf("tarDir: %w", err)
	}
	return buf.Bytes(), nil
}

func TarGZ(src string, w io.Writer) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("unable to tar files - %v", err.Error())
	}

	gw := gzip.NewWriter(w)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// include directories so extractors don’t complain, skip other non-regular types
		if fi.IsDir() {
			hdr, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, p)
			if err != nil {
				return err
			}
			if rel == "." {
				// root dir entry not necessary
				return nil
			}
			hdr.Name = rel + "/"
			return tw.WriteHeader(hdr)
		}

		if !fi.Mode().IsRegular() {
			return nil
		}

		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		hdr.Name = rel

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}

func writeVersionedMetadataFile(path, localDir string) error {
	role := strings.TrimSuffix(filepath.Base(path), ".json")

	var meta struct{ Signed struct{ Version int64 } }
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &meta); err != nil {
		return err
	}

	versioned := filepath.Join(localDir, fmt.Sprintf("%d.%s.json", meta.Signed.Version, role))
	return os.WriteFile(versioned, b, 0o644)
}
