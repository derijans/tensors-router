package update

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
	"tensors-router/internal/hardware"
)

func TestReleaseFromSignedManifestSelectsExactAuthorizedPayload(t *testing.T) {
	target := testConfig(t)
	download := NewManager(target).targets()[0]
	download.Source.RepositoryURL = "https://github.com/owner/repository"
	download.Source.AssetGlob = "kobold-*.zip"
	expected := resolvedPayload{
		Name:   "kobold-linux.zip",
		URL:    "https://downloads.example.test/kobold-linux.zip",
		Length: 1234,
		SHA256: strings.Repeat("a", 64),
	}
	content := marshalSignedManifest(t, signedReleaseManifest{
		Schema:     1,
		Repository: "https://github.com/owner/repository/",
		Releases: []signedManifestRelease{{
			ID:          "stable-id",
			Tag:         "stable-tag",
			PublishedAt: time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
			Payloads:    []signedManifestPayload{{Name: expected.Name, URL: expected.URL, Length: expected.Length, SHA256: expected.SHA256}},
		}},
	})

	release, err := releaseFromSignedManifest(content, download, hardware.Info{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if release.ID != "stable-id" || release.Tag != "stable-tag" || !reflect.DeepEqual(release.Payloads, []resolvedPayload{expected}) {
		t.Fatalf("unexpected authorized release %#v", release)
	}
}

func TestReleaseFromSignedManifestPrereleasePolicy(t *testing.T) {
	cfg := testConfig(t)
	target := NewManager(cfg).targets()[0]
	target.Source.RepositoryURL = "https://github.com/owner/repository"
	target.Source.AssetGlob = "kobold-*"
	stable := signedManifestRelease{
		ID:          "stable",
		Tag:         "stable",
		PublishedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Payloads:    []signedManifestPayload{validSignedPayload("kobold-stable")},
	}
	preview := signedManifestRelease{
		ID:          "preview",
		Tag:         "preview",
		PublishedAt: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		Prerelease:  true,
		Payloads:    []signedManifestPayload{validSignedPayload("kobold-preview")},
	}
	content := marshalSignedManifest(t, signedReleaseManifest{Schema: 1, Repository: target.Source.RepositoryURL, Releases: []signedManifestRelease{stable, preview}})

	withoutPrerelease, err := releaseFromSignedManifest(content, target, hardware.Info{}, false)
	if err != nil {
		t.Fatal(err)
	}
	withPrerelease, err := releaseFromSignedManifest(content, target, hardware.Info{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if withoutPrerelease.ID != "stable" || withPrerelease.ID != "preview" {
		t.Fatalf("unexpected prerelease selection stable=%#v preview=%#v", withoutPrerelease, withPrerelease)
	}
}

func TestReleaseFromSignedManifestRejectsUnauthorizedOrIncompletePayloads(t *testing.T) {
	cfg := testConfig(t)
	target := NewManager(cfg).targets()[0]
	target.Source.RepositoryURL = "https://github.com/owner/repository"
	target.Source.AssetGlob = "kobold-*"
	tests := []struct {
		name       string
		repository string
		payload    signedManifestPayload
	}{
		{name: "repository", repository: "https://github.com/other/repository", payload: validSignedPayload("kobold-valid")},
		{name: "length", repository: target.Source.RepositoryURL, payload: signedManifestPayload{Name: "kobold-invalid", URL: "https://downloads.example.test/kobold-invalid", Length: 0, SHA256: strings.Repeat("a", 64)}},
		{name: "digest", repository: target.Source.RepositoryURL, payload: signedManifestPayload{Name: "kobold-invalid", URL: "https://downloads.example.test/kobold-invalid", Length: 1, SHA256: "invalid"}},
		{name: "url", repository: target.Source.RepositoryURL, payload: signedManifestPayload{Name: "kobold-invalid", URL: "http://downloads.example.test/kobold-invalid", Length: 1, SHA256: strings.Repeat("a", 64)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := marshalSignedManifest(t, signedReleaseManifest{
				Schema:     1,
				Repository: test.repository,
				Releases: []signedManifestRelease{{
					ID:          "release",
					Tag:         "release",
					PublishedAt: time.Now().UTC(),
					Payloads:    []signedManifestPayload{test.payload},
				}},
			})
			if _, err := releaseFromSignedManifest(content, target, hardware.Info{}, false); err == nil {
				t.Fatal("expected signed manifest rejection")
			}
		})
	}
}

func TestFailedTrustedMetadataRefreshPreservesCurrentCache(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	cfg := testConfig(t)
	cfg.Updates.BinaryURL = ""
	cfg.Updates.BinaryRepositoryURL = "https://github.com/LostRuins/koboldcpp"
	cfg.Updates.TUFRepositoryURL = server.URL + "/metadata"
	manager := NewManager(cfg)
	manager.client = server.Client()
	target := manager.targets()[0]
	cacheRoot := filepath.Join(target.DataDir, "tuf", target.Name)
	currentMetadataDir := filepath.Join(cacheRoot, "current", "metadata")
	if err := os.MkdirAll(currentMetadataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(currentMetadataDir, "sentinel.json")
	sentinel := []byte(`{"trusted":"unchanged"}`)
	if err := os.WriteFile(sentinelPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := manager.prepareTrustedRelease(context.Background(), target, hardware.Info{}); err == nil {
		t.Fatal("expected trusted metadata refresh failure")
	}
	content, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(content, sentinel) {
		t.Fatalf("current trusted cache changed after failed refresh: %q", content)
	}
	stagingDirectories, err := filepath.Glob(filepath.Join(cacheRoot, ".refresh-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stagingDirectories) != 0 {
		t.Fatalf("failed refresh left staging directories %#v on %s/%s", stagingDirectories, runtime.GOOS, runtime.GOARCH)
	}
}

func TestTrustedMetadataRejectsInvalidOrExpiredRoot(t *testing.T) {
	for _, test := range []struct {
		name    string
		expires time.Time
		tamper  bool
	}{
		{name: "invalid signature", expires: time.Now().UTC().Add(time.Hour), tamper: true},
		{name: "expired", expires: time.Now().UTC().Add(-time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := signedTestRoot(t, test.expires)
			if test.tamper {
				root.Signatures[0].Signature[0] ^= 0xff
			}
			content, err := root.ToBytes(true)
			if err != nil {
				t.Fatal(err)
			}
			cfg := testConfig(t)
			cfg.Updates.BinaryURL = ""
			cfg.Updates.BinaryRepositoryURL = "https://github.com/LostRuins/koboldcpp"
			cfg.Updates.TUFRootPath = filepath.Join(t.TempDir(), "root.json")
			if err := os.WriteFile(cfg.Updates.TUFRootPath, content, 0o600); err != nil {
				t.Fatal(err)
			}
			manager := NewManager(cfg)
			if _, _, err := manager.prepareTrustedRelease(context.Background(), manager.targets()[0], hardware.Info{}); err == nil {
				t.Fatal("expected trusted root rejection")
			}
		})
	}
}

func TestTrustedRefreshRejectsRollbackAndMixAndMatchWithoutChangingCache(t *testing.T) {
	keys := newRefreshKeys(t)
	root := refreshRoot(t, keys)
	rootBytes, err := root.ToBytes(true)
	if err != nil {
		t.Fatal(err)
	}
	rootPath := filepath.Join(t.TempDir(), "root.json")
	if err := os.WriteFile(rootPath, rootBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var served map[string][]byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, ok := served[request.URL.Path]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(content)
	}))
	defer server.Close()
	cfg := testConfig(t)
	cfg.Updates.BinaryURL = ""
	cfg.Updates.BinaryRepositoryURL = "https://github.com/LostRuins/koboldcpp"
	cfg.Updates.BinaryAssetGlob = "kobold-test"
	cfg.Updates.TUFRootPath = rootPath
	cfg.Updates.TUFRepositoryURL = server.URL + "/metadata"
	manager := NewManager(cfg)
	manager.client = server.Client()
	target := manager.targets()[0]
	served = refreshRepository(t, keys, target, server.URL, 2, false)
	_, prepared, err := manager.prepareTrustedRelease(context.Background(), target, hardware.Info{})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := promoteDirectory(prepared.stagingDir, prepared.currentDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := promotion.Commit(); err != nil {
		t.Fatal(err)
	}
	cacheBefore := readDirectoryFiles(t, filepath.Join(target.DataDir, "tuf", target.Name, "current", "metadata"))
	for _, attempt := range []struct {
		name        string
		version     int64
		mixAndMatch bool
	}{
		{name: "rollback", version: 1},
		{name: "mix and match", version: 3, mixAndMatch: true},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			served = refreshRepository(t, keys, target, server.URL, attempt.version, attempt.mixAndMatch)
			if _, _, err := manager.prepareTrustedRelease(context.Background(), target, hardware.Info{}); err == nil {
				t.Fatal("expected trusted refresh rejection")
			}
			cacheAfter := readDirectoryFiles(t, filepath.Join(target.DataDir, "tuf", target.Name, "current", "metadata"))
			if !reflect.DeepEqual(cacheAfter, cacheBefore) {
				t.Fatal("rejected trusted refresh changed the current metadata cache")
			}
		})
	}
}

type refreshKeys struct {
	root, targets, snapshot, timestamp ed25519.PrivateKey
}

func newRefreshKeys(t *testing.T) refreshKeys {
	t.Helper()
	generate := func() ed25519.PrivateKey {
		_, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		return private
	}
	return refreshKeys{root: generate(), targets: generate(), snapshot: generate(), timestamp: generate()}
}

func refreshRoot(t *testing.T, keys refreshKeys) *tufmetadata.Metadata[tufmetadata.RootType] {
	t.Helper()
	root := tufmetadata.Root(time.Now().UTC().AddDate(1, 0, 0))
	root.Signed.ConsistentSnapshot = false
	for role, private := range map[string]ed25519.PrivateKey{"root": keys.root, "targets": keys.targets, "snapshot": keys.snapshot, "timestamp": keys.timestamp} {
		key, err := tufmetadata.KeyFromPublicKey(private.Public())
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Signed.AddKey(key, role); err != nil {
			t.Fatal(err)
		}
	}
	signRefreshMetadata(t, root, keys.root)
	return root
}

func refreshRepository(t *testing.T, keys refreshKeys, target downloadTarget, serverURL string, version int64, mixAndMatch bool) map[string][]byte {
	t.Helper()
	payload := []byte("x")
	payloadDigest := sha256.Sum256(payload)
	manifest := marshalSignedManifest(t, signedReleaseManifest{Schema: 1, Repository: target.Source.RepositoryURL, Releases: []signedManifestRelease{{ID: "release", Tag: "release", PublishedAt: time.Now().UTC(), Payloads: []signedManifestPayload{{Name: "kobold-test", URL: serverURL + "/payload", Length: int64(len(payload)), SHA256: hex.EncodeToString(payloadDigest[:])}}}}})
	manifestPath := "upstreams/" + target.Name + "/" + runtime.GOOS + "-" + runtime.GOARCH + ".json"
	targets := tufmetadata.Targets(time.Now().UTC().Add(time.Hour))
	targets.Signed.Version = version
	info, err := tufmetadata.TargetFile().FromBytes(manifestPath, manifest, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	targets.Signed.Targets[manifestPath] = info
	signRefreshMetadata(t, targets, keys.targets)
	targetsBytes, err := targets.ToBytes(true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tufmetadata.Snapshot(time.Now().UTC().Add(time.Hour))
	snapshot.Signed.Version = version
	snapshot.Signed.Meta["targets.json"] = refreshMetaFile(version, targetsBytes)
	signRefreshMetadata(t, snapshot, keys.snapshot)
	snapshotBytes, err := snapshot.ToBytes(true)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := tufmetadata.Timestamp(time.Now().UTC().Add(time.Hour))
	timestamp.Signed.Version = version
	timestamp.Signed.Meta["snapshot.json"] = refreshMetaFile(version, snapshotBytes)
	signRefreshMetadata(t, timestamp, keys.timestamp)
	timestampBytes, err := timestamp.ToBytes(true)
	if err != nil {
		t.Fatal(err)
	}
	if mixAndMatch {
		snapshotBytes[0] ^= 0x01
	}
	return map[string][]byte{"/metadata/timestamp.json": timestampBytes, "/metadata/snapshot.json": snapshotBytes, "/metadata/targets.json": targetsBytes, "/targets/" + manifestPath: manifest, "/payload": payload}
}

func refreshMetaFile(version int64, body []byte) *tufmetadata.MetaFiles {
	digest := sha256.Sum256(body)
	return &tufmetadata.MetaFiles{Version: version, Length: int64(len(body)), Hashes: tufmetadata.Hashes{"sha256": digest[:]}}
}

func signRefreshMetadata[T tufmetadata.RootType | tufmetadata.TargetsType | tufmetadata.SnapshotType | tufmetadata.TimestampType](t *testing.T, value *tufmetadata.Metadata[T], private ed25519.PrivateKey) {
	t.Helper()
	signer, err := signature.LoadSigner(private, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := value.Sign(signer); err != nil {
		t.Fatal(err)
	}
}

func readDirectoryFiles(t *testing.T, directory string) map[string]string {
	t.Helper()
	result := map[string]string{}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result[entry.Name()] = string(content)
	}
	return result
}

func signedTestRoot(t *testing.T, expires time.Time) *tufmetadata.Metadata[tufmetadata.RootType] {
	t.Helper()
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := tufmetadata.KeyFromPublicKey(private.Public())
	if err != nil {
		t.Fatal(err)
	}
	root := tufmetadata.Root(expires)
	for _, role := range []string{"root", "targets", "snapshot", "timestamp"} {
		if err := root.Signed.AddKey(key, role); err != nil {
			t.Fatal(err)
		}
	}
	signer, err := signature.LoadSigner(private, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Sign(signer); err != nil {
		t.Fatal(err)
	}
	return root
}

func validSignedPayload(name string) signedManifestPayload {
	return signedManifestPayload{Name: name, URL: "https://downloads.example.test/" + name, Length: 1, SHA256: strings.Repeat("a", 64)}
}

func marshalSignedManifest(t *testing.T, manifest signedReleaseManifest) []byte {
	t.Helper()
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
