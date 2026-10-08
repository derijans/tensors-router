package tufpublish

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tensors-router/internal/atomicfile"

	"github.com/sigstore/sigstore/pkg/signature"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
	tufconfig "github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

type SigningSecrets struct{ Upstream, Snapshot, Timestamp string }

const (
	maxTrustedRootBytes       = 1 << 20
	maxTimestampMetadataBytes = 1 << 20
	maxSnapshotMetadataBytes  = 4 << 20
	maxTargetsMetadataBytes   = 16 << 20
	maxVerificationFileBytes  = 32 << 20
)

type directoryFetcher struct{ root string }

func (fetcher directoryFetcher) DownloadFile(rawURL string, maxLength int64, _ time.Duration) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	relative := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/")))
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("unsafe verification path")
	}
	maximum := int64(maxLength)
	if maximum > maxVerificationFileBytes {
		maximum = maxVerificationFileBytes
	}
	content, err := readBoundedFile(filepath.Join(fetcher.root, relative), maximum)
	if os.IsNotExist(err) {
		return nil, &tufmetadata.ErrDownloadHTTP{StatusCode: 404, URL: rawURL}
	}
	if err != nil {
		return nil, err
	}
	return content, nil
}

func Verify(repository string, expectedTargets []string) error {
	root, err := readBoundedFile(filepath.Join(repository, "metadata", "root.json"), maxTrustedRootBytes)
	if err != nil {
		return err
	}
	configuration, err := tufconfig.New("https://verification.invalid/metadata", root)
	if err != nil {
		return err
	}
	local, err := os.MkdirTemp("", "tuf-publication-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(local)
	configuration.LocalMetadataDir = filepath.Join(local, "metadata")
	configuration.LocalTargetsDir = filepath.Join(local, "targets")
	configuration.RemoteTargetsURL = "https://verification.invalid/targets"
	configuration.Fetcher = directoryFetcher{root: repository}
	client, err := updater.New(configuration)
	if err != nil {
		return err
	}
	if err := client.Refresh(); err != nil {
		return fmt.Errorf("clean-cache publication verification: %w", err)
	}
	downloadDirectory := filepath.Join(local, "downloads")
	if err := os.MkdirAll(downloadDirectory, 0o700); err != nil {
		return err
	}
	for _, targetPath := range expectedTargets {
		info, err := client.GetTargetInfo(targetPath)
		if err != nil {
			return err
		}
		if _, _, err := client.DownloadTarget(info, filepath.Join(downloadDirectory, url.PathEscape(targetPath)), ""); err != nil {
			return err
		}
	}
	return nil
}

func LoadExistingTargets(repository string, prefix string) (map[string][]byte, error) {
	if prefix == "" || strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "..") {
		return nil, fmt.Errorf("existing target prefix is invalid")
	}
	delegatedBytes, err := readBoundedFile(filepath.Join(repository, "metadata", "upstream-targets.json"), maxTargetsMetadataBytes)
	if err != nil {
		return nil, err
	}
	delegated, err := tufmetadata.Targets().FromBytes(delegatedBytes)
	if err != nil {
		return nil, err
	}
	candidates := make([]string, 0)
	for targetPath := range delegated.Signed.Targets {
		if strings.HasPrefix(targetPath, prefix) {
			candidates = append(candidates, targetPath)
		}
	}
	if len(candidates) == 0 {
		return map[string][]byte{}, nil
	}
	root, err := readBoundedFile(filepath.Join(repository, "metadata", "root.json"), maxTrustedRootBytes)
	if err != nil {
		return nil, err
	}
	configuration, err := tufconfig.New("https://verification.invalid/metadata", root)
	if err != nil {
		return nil, err
	}
	local, err := os.MkdirTemp("", "tuf-existing-targets-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(local)
	configuration.LocalMetadataDir = filepath.Join(local, "metadata")
	configuration.LocalTargetsDir = filepath.Join(local, "targets")
	configuration.RemoteTargetsURL = "https://verification.invalid/targets"
	configuration.Fetcher = directoryFetcher{root: repository}
	client, err := updater.New(configuration)
	if err != nil {
		return nil, err
	}
	if err := client.Refresh(); err != nil {
		return nil, fmt.Errorf("refresh existing publication: %w", err)
	}
	sort.Strings(candidates)
	bodies := make(map[string][]byte, len(candidates))
	for _, targetPath := range candidates {
		info, err := client.GetTargetInfo(targetPath)
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(local, "downloads", url.PathEscape(targetPath))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return nil, err
		}
		path, _, err := client.DownloadTarget(info, destination, "")
		if err != nil {
			return nil, err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		bodies[targetPath] = body
	}
	return bodies, nil
}

func delegatedRole(targets *tufmetadata.Metadata[tufmetadata.TargetsType], name string) (*tufmetadata.DelegatedRole, error) {
	if targets.Signed.Delegations != nil {
		for index := range targets.Signed.Delegations.Roles {
			role := &targets.Signed.Delegations.Roles[index]
			if role.Name == name && role.Threshold == 1 {
				return role, nil
			}
		}
	}
	return nil, fmt.Errorf("trusted targets do not authorize %s", name)
}

func authorizedSigner(encoded string, authorizedIDs []string) (signature.Signer, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid base64 Ed25519 private key")
	}
	private := ed25519.PrivateKey(raw)
	key, err := tufmetadata.KeyFromPublicKey(private.Public())
	if err != nil {
		return nil, err
	}
	keyID, err := key.ID()
	if err != nil {
		return nil, err
	}
	for _, authorizedID := range authorizedIDs {
		if authorizedID == keyID {
			return signature.LoadSigner(private, crypto.Hash(0))
		}
	}
	return nil, fmt.Errorf("key %s is not authorized by trusted metadata", keyID)
}

func publicationMeta(version int64, body []byte) *tufmetadata.MetaFiles {
	digest := sha256.Sum256(body)
	return &tufmetadata.MetaFiles{Version: version, Length: int64(len(body)), Hashes: tufmetadata.Hashes{"sha256": digest[:]}}
}

func preparePublicationOutput(repository, output string) error {
	if entries, err := os.ReadDir(output); err == nil && len(entries) != 0 {
		return fmt.Errorf("output directory must be empty")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "metadata"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "targets"), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(repository, "metadata"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.Contains(name, "upstream-targets") || strings.Contains(name, "snapshot") || name == "timestamp.json" {
			continue
		}
		body, err := readBoundedFile(filepath.Join(repository, "metadata", name), maxTargetsMetadataBytes)
		if err != nil {
			return err
		}
		if err := atomicfile.Write(filepath.Join(output, "metadata", name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readBoundedFile(path string, maximum int64) ([]byte, error) {
	if maximum < 1 {
		return nil, fmt.Errorf("invalid maximum file length")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, fmt.Errorf("file %s exceeds maximum length", path)
	}
	return body, nil
}
