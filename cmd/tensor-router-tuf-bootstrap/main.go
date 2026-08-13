package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"

	"tensors-router/internal/atomicfile"
)

type generatedKey struct {
	private ed25519.PrivateKey
	public  *tufmetadata.Key
}

func main() {
	keyDirectory := flag.String("key-dir", ".tmp/tuf-bootstrap", "private key custody directory")
	rootOutput := flag.String("root-output", "internal/update/trusted-root.json", "public root metadata output")
	flag.Parse()
	if err := bootstrap(*keyDirectory, *rootOutput); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bootstrap(keyDirectory string, rootOutput string) error {
	if entries, err := os.ReadDir(keyDirectory); err == nil && len(entries) != 0 {
		return fmt.Errorf("bootstrap key directory %s is not empty", keyDirectory)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(keyDirectory, 0o700); err != nil {
		return err
	}
	if err := secureDirectory(keyDirectory); err != nil {
		return err
	}
	roleCounts := map[string]int{"root": 3, "targets": 2, "snapshot": 1, "timestamp": 1, "upstream-targets": 1}
	keys := map[string][]generatedKey{}
	for _, role := range []string{"root", "targets", "snapshot", "timestamp", "upstream-targets"} {
		for index := 0; index < roleCounts[role]; index++ {
			key, err := generateKey()
			if err != nil {
				return err
			}
			keys[role] = append(keys[role], key)
			name := fmt.Sprintf("%s-%d.ed25519.base64", role, index+1)
			encoded := []byte(base64.StdEncoding.EncodeToString(key.private))
			if err := atomicfile.Write(filepath.Join(keyDirectory, name), encoded, 0o600); err != nil {
				return err
			}
		}
	}
	root := tufmetadata.Root(time.Now().UTC().AddDate(10, 0, 0))
	root.Signed.Version = 1
	root.Signed.ConsistentSnapshot = true
	for _, role := range []string{"root", "targets", "snapshot", "timestamp"} {
		roleKeys := keys[role]
		for _, key := range roleKeys {
			if err := root.Signed.AddKey(key.public, role); err != nil {
				return err
			}
		}
	}
	root.Signed.Roles["root"].Threshold = 2
	root.Signed.Roles["targets"].Threshold = 2
	for _, key := range keys["root"][:2] {
		signer, err := signature.LoadSigner(key.private, crypto.Hash(0))
		if err != nil {
			return err
		}
		if _, err := root.Sign(signer); err != nil {
			return err
		}
	}
	content, err := root.ToBytes(true)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(rootOutput, content, 0o644); err != nil {
		return err
	}
	if err := writeInitialRepository(keyDirectory, root, keys); err != nil {
		return err
	}
	recovery := []byte("Keep root-1 through root-3 and targets-1 through targets-2 in separate offline custody. Provision upstream-targets-1, snapshot-1, and timestamp-1 to protected publication secrets. Publish repository/ as the initial public repository. Do not commit or print private key files.\\n")
	return atomicfile.Write(filepath.Join(keyDirectory, "CUSTODY.txt"), recovery, 0o600)
}

func writeInitialRepository(keyDirectory string, root *tufmetadata.Metadata[tufmetadata.RootType], keys map[string][]generatedKey) error {
	metadataDirectory := filepath.Join(keyDirectory, "repository", "metadata")
	if err := os.MkdirAll(metadataDirectory, 0o700); err != nil {
		return err
	}
	delegatedKey := keys["upstream-targets"][0]
	delegatedKeyID, err := delegatedKey.public.ID()
	if err != nil {
		return err
	}
	targets := tufmetadata.Targets(time.Now().UTC().AddDate(0, 6, 0))
	targets.Signed.Delegations = &tufmetadata.Delegations{
		Keys:  map[string]*tufmetadata.Key{delegatedKeyID: delegatedKey.public},
		Roles: []tufmetadata.DelegatedRole{{Name: "upstream-targets", KeyIDs: []string{delegatedKeyID}, Threshold: 1, Terminating: true, Paths: []string{"upstreams/*/*", "runtimes/vllm/*"}}},
	}
	for _, key := range keys["targets"] {
		if err := signMetadata(targets, key.private); err != nil {
			return err
		}
	}
	delegated := tufmetadata.Targets(time.Now().UTC().AddDate(0, 1, 0))
	if err := signMetadata(delegated, delegatedKey.private); err != nil {
		return err
	}
	targetsBytes, err := targets.ToBytes(true)
	if err != nil {
		return err
	}
	delegatedBytes, err := delegated.ToBytes(true)
	if err != nil {
		return err
	}
	snapshot := tufmetadata.Snapshot(time.Now().UTC().AddDate(0, 0, 14))
	snapshot.Signed.Meta["targets.json"] = metadataFile(targets.Signed.Version, targetsBytes)
	snapshot.Signed.Meta["upstream-targets.json"] = metadataFile(delegated.Signed.Version, delegatedBytes)
	if err := signMetadata(snapshot, keys["snapshot"][0].private); err != nil {
		return err
	}
	snapshotBytes, err := snapshot.ToBytes(true)
	if err != nil {
		return err
	}
	timestamp := tufmetadata.Timestamp(time.Now().UTC().AddDate(0, 0, 2))
	timestamp.Signed.Meta["snapshot.json"] = metadataFile(snapshot.Signed.Version, snapshotBytes)
	if err := signMetadata(timestamp, keys["timestamp"][0].private); err != nil {
		return err
	}
	timestampBytes, err := timestamp.ToBytes(true)
	if err != nil {
		return err
	}
	rootBytes, err := root.ToBytes(true)
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"1.root.json":             rootBytes,
		"root.json":               rootBytes,
		"1.targets.json":          targetsBytes,
		"targets.json":            targetsBytes,
		"1.upstream-targets.json": delegatedBytes,
		"upstream-targets.json":   delegatedBytes,
		"1.snapshot.json":         snapshotBytes,
		"snapshot.json":           snapshotBytes,
		"timestamp.json":          timestampBytes,
	}
	for name, body := range files {
		if err := atomicfile.Write(filepath.Join(metadataDirectory, name), body, 0o644); err != nil {
			return err
		}
	}
	return os.MkdirAll(filepath.Join(keyDirectory, "repository", "targets"), 0o700)
}

func metadataFile(version int64, content []byte) *tufmetadata.MetaFiles {
	digest := sha256.Sum256(content)
	return &tufmetadata.MetaFiles{Version: version, Length: int64(len(content)), Hashes: tufmetadata.Hashes{"sha256": digest[:]}}
}

func signMetadata[T tufmetadata.RootType | tufmetadata.TargetsType | tufmetadata.SnapshotType | tufmetadata.TimestampType](value *tufmetadata.Metadata[T], private ed25519.PrivateKey) error {
	signer, err := signature.LoadSigner(private, crypto.Hash(0))
	if err != nil {
		return err
	}
	_, err = value.Sign(signer)
	return err
}

func generateKey() (generatedKey, error) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		return generatedKey{}, err
	}
	public, err := tufmetadata.KeyFromPublicKey(private.Public())
	if err != nil {
		return generatedKey{}, err
	}
	return generatedKey{private: private, public: public}, nil
}
