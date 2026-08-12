package tufpublish

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
	tufconfig "github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
	"tensors-router/internal/atomicfile"
)

type SigningSecrets struct{ Upstream, Snapshot, Timestamp string }

func Publish(repository, output string, targetBodies map[string][]byte, secrets SigningSecrets, now time.Time) error {
	metadataDir := filepath.Join(repository, "metadata")
	root, err := tufmetadata.Root().FromFile(filepath.Join(metadataDir, "root.json"))
	if err != nil {
		return err
	}
	top, err := tufmetadata.Targets().FromFile(filepath.Join(metadataDir, "targets.json"))
	if err != nil {
		return err
	}
	oldDelegated, err := tufmetadata.Targets().FromFile(filepath.Join(metadataDir, "upstream-targets.json"))
	if err != nil {
		return err
	}
	oldSnapshot, err := tufmetadata.Snapshot().FromFile(filepath.Join(metadataDir, "snapshot.json"))
	if err != nil {
		return err
	}
	oldTimestamp, err := tufmetadata.Timestamp().FromFile(filepath.Join(metadataDir, "timestamp.json"))
	if err != nil {
		return err
	}
	if root.Signed.IsExpired(now) {
		return fmt.Errorf("root metadata is expired")
	}
	upstreamRole, err := delegatedRole(top, "upstream-targets")
	if err != nil {
		return err
	}
	for targetPath := range targetBodies {
		allowed, err := upstreamRole.IsDelegatedPath(targetPath)
		if err != nil {
			return fmt.Errorf("validate delegated target path %q: %w", targetPath, err)
		}
		if !allowed {
			return fmt.Errorf("trusted targets do not delegate %q to upstream-targets", targetPath)
		}
	}
	upstreamSigner, err := authorizedSigner(secrets.Upstream, upstreamRole.KeyIDs)
	if err != nil {
		return fmt.Errorf("upstream-targets key: %w", err)
	}
	snapshotSigner, err := authorizedSigner(secrets.Snapshot, root.Signed.Roles["snapshot"].KeyIDs)
	if err != nil {
		return fmt.Errorf("snapshot key: %w", err)
	}
	timestampSigner, err := authorizedSigner(secrets.Timestamp, root.Signed.Roles["timestamp"].KeyIDs)
	if err != nil {
		return fmt.Errorf("timestamp key: %w", err)
	}
	delegated := tufmetadata.Targets(now.AddDate(0, 1, 0))
	delegated.Signed.Version = oldDelegated.Signed.Version + 1
	for targetPath, body := range targetBodies {
		info, err := tufmetadata.TargetFile().FromBytes(targetPath, body, "sha256")
		if err != nil {
			return err
		}
		delegated.Signed.Targets[targetPath] = info
	}
	if _, err := delegated.Sign(upstreamSigner); err != nil {
		return err
	}
	delegatedBytes, err := delegated.ToBytes(true)
	if err != nil {
		return err
	}
	topBytes, err := os.ReadFile(filepath.Join(metadataDir, "targets.json"))
	if err != nil {
		return err
	}
	snapshot := tufmetadata.Snapshot(now.AddDate(0, 0, 14))
	snapshot.Signed.Version = oldSnapshot.Signed.Version + 1
	snapshot.Signed.Meta["targets.json"] = publicationMeta(top.Signed.Version, topBytes)
	snapshot.Signed.Meta["upstream-targets.json"] = publicationMeta(delegated.Signed.Version, delegatedBytes)
	if _, err := snapshot.Sign(snapshotSigner); err != nil {
		return err
	}
	snapshotBytes, err := snapshot.ToBytes(true)
	if err != nil {
		return err
	}
	timestamp := tufmetadata.Timestamp(now.AddDate(0, 0, 2))
	timestamp.Signed.Version = oldTimestamp.Signed.Version + 1
	timestamp.Signed.Meta["snapshot.json"] = publicationMeta(snapshot.Signed.Version, snapshotBytes)
	if _, err := timestamp.Sign(timestampSigner); err != nil {
		return err
	}
	timestampBytes, err := timestamp.ToBytes(true)
	if err != nil {
		return err
	}
	if err := preparePublicationOutput(repository, output); err != nil {
		return err
	}
	metadataOutputs := map[string][]byte{
		fmt.Sprintf("%d.upstream-targets.json", delegated.Signed.Version): delegatedBytes,
		"upstream-targets.json":                                  delegatedBytes,
		fmt.Sprintf("%d.snapshot.json", snapshot.Signed.Version): snapshotBytes,
		"snapshot.json":                                          snapshotBytes,
	}
	for name, body := range metadataOutputs {
		if err := atomicfile.Write(filepath.Join(output, "metadata", name), body, 0o644); err != nil {
			return err
		}
	}
	for targetPath, body := range targetBodies {
		digest := sha256.Sum256(body)
		directory, name := filepath.Split(filepath.FromSlash(targetPath))
		name = hex.EncodeToString(digest[:]) + "." + name
		if err := atomicfile.Write(filepath.Join(output, "targets", directory, name), body, 0o644); err != nil {
			return err
		}
	}
	if err := atomicfile.Write(filepath.Join(output, "metadata", "timestamp.json"), timestampBytes, 0o644); err != nil {
		return err
	}
	expectedTargets := make([]string, 0, len(targetBodies))
	for targetPath := range targetBodies {
		expectedTargets = append(expectedTargets, targetPath)
	}
	return Verify(output, expectedTargets)
}

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
	content, err := os.ReadFile(filepath.Join(fetcher.root, relative))
	if os.IsNotExist(err) {
		return nil, &tufmetadata.ErrDownloadHTTP{StatusCode: 404, URL: rawURL}
	}
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxLength {
		return nil, fmt.Errorf("verification file exceeds trusted length")
	}
	return content, nil
}

func Verify(repository string, expectedTargets []string) error {
	root, err := os.ReadFile(filepath.Join(repository, "metadata", "root.json"))
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
	delegated, err := tufmetadata.Targets().FromFile(filepath.Join(repository, "metadata", "upstream-targets.json"))
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
	root, err := os.ReadFile(filepath.Join(repository, "metadata", "root.json"))
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
		body, err := os.ReadFile(filepath.Join(repository, "metadata", name))
		if err != nil {
			return err
		}
		if err := atomicfile.Write(filepath.Join(output, "metadata", name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
