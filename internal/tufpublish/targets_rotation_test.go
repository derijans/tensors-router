package tufpublish

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"

	"tensors-router/internal/atomicfile"
)

func TestTargetsRotationAddsVLLMPathWithOfflineThreshold(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository, targetsKeys, secrets := rotationTestRepository(t, now)
	requestDirectory := filepath.Join(t.TempDir(), "request")
	expires := now.AddDate(0, 6, 0)
	if err := PrepareTargetsRotation(repository, requestDirectory, expires, now); err != nil {
		t.Fatal(err)
	}
	payload := readRotationTestFile(t, filepath.Join(requestDirectory, "targets.payload.json"))
	unsignedPath := filepath.Join(requestDirectory, "targets.unsigned.json")
	unsigned := readRotationTestFile(t, unsignedPath)
	candidate, err := tufmetadata.Targets().FromBytes(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	canonicalPayload, err := cjson.EncodeCanonical(candidate.Signed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, canonicalPayload) {
		t.Fatal("signing payload is not canonical targets metadata")
	}
	requestBytes := readRotationTestFile(t, filepath.Join(requestDirectory, "rotation-request.json"))
	var request TargetsRotationRequest
	if err := json.Unmarshal(requestBytes, &request); err != nil {
		t.Fatal(err)
	}
	canonicalRequest, err := cjson.EncodeCanonical(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(requestBytes, canonicalRequest) {
		t.Fatal("rotation request is not canonical JSON")
	}
	if request.RequiredSignatures != 2 || request.SourceVersion != 1 || request.CandidateVersion != 2 {
		t.Fatalf("unexpected rotation request: %+v", request)
	}
	if !slices.Equal(request.AddedDelegatedPaths, []string{vllmDelegatedPath}) {
		t.Fatalf("unexpected added paths: %v", request.AddedDelegatedPaths)
	}
	signPublicationTest(t, candidate, targetsKeys[0])
	signPublicationTest(t, candidate, targetsKeys[1])
	signedPath := filepath.Join(t.TempDir(), "targets.json")
	if err := candidate.ToFile(signedPath, true); err != nil {
		t.Fatal(err)
	}
	if err := InstallTargetsRotation(repository, signedPath, filepath.Join(repository, "staged"), now); err == nil {
		t.Fatal("rotation accepted an output directory inside the source repository")
	}
	originalSnapshot := readRotationTestFile(t, filepath.Join(repository, "metadata", "snapshot.json"))
	originalTimestamp := readRotationTestFile(t, filepath.Join(repository, "metadata", "timestamp.json"))
	staged := filepath.Join(t.TempDir(), "staged")
	if err := InstallTargetsRotation(repository, signedPath, staged, now); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(originalSnapshot, readRotationTestFile(t, filepath.Join(staged, "metadata", "snapshot.json"))) {
		t.Fatal("offline installation changed snapshot metadata")
	}
	if !bytes.Equal(originalTimestamp, readRotationTestFile(t, filepath.Join(staged, "metadata", "timestamp.json"))) {
		t.Fatal("offline installation changed timestamp metadata")
	}
	installed, err := tufmetadata.Targets().FromFile(filepath.Join(staged, "metadata", "targets.json"))
	if err != nil {
		t.Fatal(err)
	}
	role, err := delegatedRole(installed, "upstream-targets")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(role.Paths, vllmDelegatedPath) {
		t.Fatal("installed targets metadata does not authorize vLLM runtime manifests")
	}
	published := filepath.Join(t.TempDir(), "published")
	targetPath := "runtimes/vllm/linux-amd64.json"
	if err := Publish(staged, published, map[string][]byte{targetPath: []byte(`{"schema_version":1}`)}, secrets, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Verify(published, []string{targetPath}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetsRotationRejectsInsufficientAndAlteredSignatures(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository, targetsKeys, _ := rotationTestRepository(t, now)
	requestDirectory := filepath.Join(t.TempDir(), "request")
	if err := PrepareTargetsRotation(repository, requestDirectory, now.AddDate(0, 6, 0), now); err != nil {
		t.Fatal(err)
	}
	candidate, err := tufmetadata.Targets().FromFile(filepath.Join(requestDirectory, "targets.unsigned.json"))
	if err != nil {
		t.Fatal(err)
	}
	signPublicationTest(t, candidate, targetsKeys[0])
	oneSignaturePath := filepath.Join(t.TempDir(), "one-signature.json")
	if err := candidate.ToFile(oneSignaturePath, true); err != nil {
		t.Fatal(err)
	}
	if err := InstallTargetsRotation(repository, oneSignaturePath, filepath.Join(t.TempDir(), "staged"), now); err == nil {
		t.Fatal("rotation accepted fewer signatures than the trusted threshold")
	}
	candidate.ClearSignatures()
	candidate.Signed.Targets["unapproved.json"] = &tufmetadata.TargetFiles{Length: 0, Hashes: tufmetadata.Hashes{}}
	signPublicationTest(t, candidate, targetsKeys[0])
	signPublicationTest(t, candidate, targetsKeys[1])
	alteredPath := filepath.Join(t.TempDir(), "altered.json")
	if err := candidate.ToFile(alteredPath, true); err != nil {
		t.Fatal(err)
	}
	if err := InstallTargetsRotation(repository, alteredPath, filepath.Join(t.TempDir(), "altered-staged"), now); err == nil {
		t.Fatal("rotation accepted changes outside the approved delegation path")
	}
}

func rotationTestRepository(t *testing.T, now time.Time) (string, []ed25519.PrivateKey, SigningSecrets) {
	t.Helper()
	generate := func() ed25519.PrivateKey {
		_, key, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	rootKey := generate()
	targetsKeys := []ed25519.PrivateKey{generate(), generate(), generate()}
	upstreamKey := generate()
	snapshotKey := generate()
	timestampKey := generate()
	root := tufmetadata.Root(now.AddDate(2, 0, 0))
	root.Signed.ConsistentSnapshot = true
	addRootKey := func(role string, private ed25519.PrivateKey) {
		key, err := tufmetadata.KeyFromPublicKey(private.Public())
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Signed.AddKey(key, role); err != nil {
			t.Fatal(err)
		}
	}
	addRootKey(tufmetadata.ROOT, rootKey)
	for _, key := range targetsKeys {
		addRootKey(tufmetadata.TARGETS, key)
	}
	addRootKey(tufmetadata.SNAPSHOT, snapshotKey)
	addRootKey(tufmetadata.TIMESTAMP, timestampKey)
	root.Signed.Roles[tufmetadata.TARGETS].Threshold = 2
	signPublicationTest(t, root, rootKey)
	upstreamPublic, err := tufmetadata.KeyFromPublicKey(upstreamKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	upstreamID, err := upstreamPublic.ID()
	if err != nil {
		t.Fatal(err)
	}
	targets := tufmetadata.Targets(now.AddDate(0, 3, 0))
	targets.Signed.Delegations = &tufmetadata.Delegations{
		Keys:  map[string]*tufmetadata.Key{upstreamID: upstreamPublic},
		Roles: []tufmetadata.DelegatedRole{{Name: "upstream-targets", KeyIDs: []string{upstreamID}, Threshold: 1, Terminating: true, Paths: []string{"upstreams/*/*"}}},
	}
	signPublicationTest(t, targets, targetsKeys[0])
	signPublicationTest(t, targets, targetsKeys[1])
	delegated := tufmetadata.Targets(now.AddDate(0, 1, 0))
	signPublicationTest(t, delegated, upstreamKey)
	rootBytes, _ := root.ToBytes(true)
	targetsBytes, _ := targets.ToBytes(true)
	delegatedBytes, _ := delegated.ToBytes(true)
	snapshot := tufmetadata.Snapshot(now.AddDate(0, 0, 14))
	snapshot.Signed.Meta["targets.json"] = testPublicationMeta(targets.Signed.Version, targetsBytes)
	snapshot.Signed.Meta["upstream-targets.json"] = testPublicationMeta(delegated.Signed.Version, delegatedBytes)
	signPublicationTest(t, snapshot, snapshotKey)
	snapshotBytes, _ := snapshot.ToBytes(true)
	timestamp := tufmetadata.Timestamp(now.AddDate(0, 0, 2))
	timestamp.Signed.Meta["snapshot.json"] = testPublicationMeta(snapshot.Signed.Version, snapshotBytes)
	signPublicationTest(t, timestamp, timestampKey)
	timestampBytes, _ := timestamp.ToBytes(true)
	repository := t.TempDir()
	metadataDirectory := filepath.Join(repository, "metadata")
	if err := os.MkdirAll(metadataDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"1.root.json": rootBytes, "root.json": rootBytes,
		"1.targets.json": targetsBytes, "targets.json": targetsBytes,
		"1.upstream-targets.json": delegatedBytes, "upstream-targets.json": delegatedBytes,
		"1.snapshot.json": snapshotBytes, "snapshot.json": snapshotBytes,
		"timestamp.json": timestampBytes,
	} {
		if err := atomicfile.Write(filepath.Join(metadataDirectory, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(repository, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	secrets := SigningSecrets{
		Upstream:  base64.StdEncoding.EncodeToString(upstreamKey),
		Snapshot:  base64.StdEncoding.EncodeToString(snapshotKey),
		Timestamp: base64.StdEncoding.EncodeToString(timestampKey),
	}
	return repository, targetsKeys, secrets
}

func readRotationTestFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
