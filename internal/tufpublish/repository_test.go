package tufpublish

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
	"tensors-router/internal/atomicfile"
)

func TestAuthorizedSignerRejectsMissingAndUntrustedKeys(t *testing.T) {
	_, trustedPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	trustedKey, err := tufmetadata.KeyFromPublicKey(trustedPrivate.Public())
	if err != nil {
		t.Fatal(err)
	}
	trustedID, err := trustedKey.ID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authorizedSigner("", []string{trustedID}); err == nil {
		t.Fatal("expected missing key rejection")
	}
	_, otherPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	encodedOther := base64.StdEncoding.EncodeToString(otherPrivate)
	if _, err := authorizedSigner(encodedOther, []string{trustedID}); err == nil {
		t.Fatal("expected unauthorized key rejection")
	}
	encodedTrusted := base64.StdEncoding.EncodeToString(trustedPrivate)
	if _, err := authorizedSigner(encodedTrusted, []string{trustedID}); err != nil {
		t.Fatal(err)
	}
}

func TestPublishIncrementsDelegatedSnapshotAndTimestampVersions(t *testing.T) {
	repository, secrets := generatedPublicationRepository(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	targets := map[string][]byte{"upstreams/koboldcpp/windows-amd64.json": []byte(`{"schema":1}`)}
	if err := Publish(repository, first, targets, secrets, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := Publish(first, second, targets, secrets, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertVersionIncrease(t, first, second, "upstream-targets")
	assertVersionIncrease(t, first, second, "snapshot")
	assertVersionIncrease(t, first, second, "timestamp")
}

type publicationKeys struct {
	root, targets, upstream, snapshot, timestamp ed25519.PrivateKey
}

func generatedPublicationRepository(t *testing.T) (string, SigningSecrets) {
	t.Helper()
	generate := func() ed25519.PrivateKey {
		_, key, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	keys := publicationKeys{root: generate(), targets: generate(), upstream: generate(), snapshot: generate(), timestamp: generate()}
	repository := t.TempDir()
	metadataDirectory := filepath.Join(repository, "metadata")
	if err := os.MkdirAll(metadataDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	root := tufmetadata.Root(time.Now().UTC().AddDate(1, 0, 0))
	root.Signed.ConsistentSnapshot = true
	for role, private := range map[string]ed25519.PrivateKey{"root": keys.root, "targets": keys.targets, "snapshot": keys.snapshot, "timestamp": keys.timestamp} {
		key, err := tufmetadata.KeyFromPublicKey(private.Public())
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Signed.AddKey(key, role); err != nil {
			t.Fatal(err)
		}
	}
	signPublicationTest(t, root, keys.root)
	upstreamKey, err := tufmetadata.KeyFromPublicKey(keys.upstream.Public())
	if err != nil {
		t.Fatal(err)
	}
	upstreamID, err := upstreamKey.ID()
	if err != nil {
		t.Fatal(err)
	}
	top := tufmetadata.Targets(time.Now().UTC().AddDate(1, 0, 0))
	top.Signed.Delegations = &tufmetadata.Delegations{Keys: map[string]*tufmetadata.Key{upstreamID: upstreamKey}, Roles: []tufmetadata.DelegatedRole{{Name: "upstream-targets", KeyIDs: []string{upstreamID}, Threshold: 1, Terminating: true, Paths: []string{"upstreams/*/*"}}}}
	signPublicationTest(t, top, keys.targets)
	delegated := tufmetadata.Targets(time.Now().UTC().Add(time.Hour))
	signPublicationTest(t, delegated, keys.upstream)
	rootBytes, _ := root.ToBytes(true)
	topBytes, _ := top.ToBytes(true)
	delegatedBytes, _ := delegated.ToBytes(true)
	snapshot := tufmetadata.Snapshot(time.Now().UTC().Add(time.Hour))
	snapshot.Signed.Meta["targets.json"] = testPublicationMeta(top.Signed.Version, topBytes)
	snapshot.Signed.Meta["upstream-targets.json"] = testPublicationMeta(delegated.Signed.Version, delegatedBytes)
	signPublicationTest(t, snapshot, keys.snapshot)
	snapshotBytes, _ := snapshot.ToBytes(true)
	timestamp := tufmetadata.Timestamp(time.Now().UTC().Add(time.Hour))
	timestamp.Signed.Meta["snapshot.json"] = testPublicationMeta(snapshot.Signed.Version, snapshotBytes)
	signPublicationTest(t, timestamp, keys.timestamp)
	timestampBytes, _ := timestamp.ToBytes(true)
	files := map[string][]byte{"root.json": rootBytes, "1.root.json": rootBytes, "targets.json": topBytes, "1.targets.json": topBytes, "upstream-targets.json": delegatedBytes, "1.upstream-targets.json": delegatedBytes, "snapshot.json": snapshotBytes, "1.snapshot.json": snapshotBytes, "timestamp.json": timestampBytes}
	for name, body := range files {
		if err := atomicfile.Write(filepath.Join(metadataDirectory, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repository, SigningSecrets{Upstream: base64.StdEncoding.EncodeToString(keys.upstream), Snapshot: base64.StdEncoding.EncodeToString(keys.snapshot), Timestamp: base64.StdEncoding.EncodeToString(keys.timestamp)}
}

func signPublicationTest[T tufmetadata.RootType | tufmetadata.TargetsType | tufmetadata.SnapshotType | tufmetadata.TimestampType](t *testing.T, value *tufmetadata.Metadata[T], private ed25519.PrivateKey) {
	t.Helper()
	signer, err := signature.LoadSigner(private, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := value.Sign(signer); err != nil {
		t.Fatal(err)
	}
}

func testPublicationMeta(version int64, body []byte) *tufmetadata.MetaFiles {
	digest := sha256.Sum256(body)
	return &tufmetadata.MetaFiles{Version: version, Length: int64(len(body)), Hashes: tufmetadata.Hashes{"sha256": digest[:]}}
}

func assertVersionIncrease(t *testing.T, first, second, role string) {
	t.Helper()
	var firstVersion, secondVersion int64
	switch role {
	case "upstream-targets":
		firstMetadata, err := tufmetadata.Targets().FromFile(filepath.Join(first, "metadata", role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		secondMetadata, err := tufmetadata.Targets().FromFile(filepath.Join(second, "metadata", role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		firstVersion, secondVersion = firstMetadata.Signed.Version, secondMetadata.Signed.Version
	case "snapshot":
		firstMetadata, err := tufmetadata.Snapshot().FromFile(filepath.Join(first, "metadata", role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		secondMetadata, err := tufmetadata.Snapshot().FromFile(filepath.Join(second, "metadata", role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		firstVersion, secondVersion = firstMetadata.Signed.Version, secondMetadata.Signed.Version
	case "timestamp":
		firstMetadata, err := tufmetadata.Timestamp().FromFile(filepath.Join(first, "metadata", role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		secondMetadata, err := tufmetadata.Timestamp().FromFile(filepath.Join(second, "metadata", role+".json"))
		if err != nil {
			t.Fatal(err)
		}
		firstVersion, secondVersion = firstMetadata.Signed.Version, secondMetadata.Signed.Version
	}
	if secondVersion != firstVersion+1 {
		t.Fatalf("%s version did not increase monotonically: %d -> %d", role, firstVersion, secondVersion)
	}
}
