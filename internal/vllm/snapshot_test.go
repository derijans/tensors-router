package vllm

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSnapshotDigestIsStableAndDetectsMutation(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "weights"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "weights", "model.bin"), []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := ComputeSnapshotDigest(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ComputeSnapshotDigest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.TreeSHA256 != second.TreeSHA256 || len(first.Files) != 2 {
		t.Fatalf("unstable digest first=%#v second=%#v", first, second)
	}
	identity := SnapshotIdentity{Path: directory, TreeDigest: first.TreeSHA256}
	if err := VerifySnapshot(identity); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifySnapshot(identity); err == nil {
		t.Fatal("expected immutable digest mismatch")
	}
}

func TestSnapshotRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged Windows symlink creation is unavailable")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ComputeSnapshotDigest(directory); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
