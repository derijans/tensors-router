package loadcapture_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/loadcapture"
)

func TestBuildSnapshotReusesCatalogHashUntilFileSizeOrModTimeChanges(t *testing.T) {
	hashes, err := catalog.NewWithStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modelDir := t.TempDir()
	model := filepath.Join(modelDir, "model.gguf")
	configPath := writeModelConfig(t, modelDir, model)
	writeModelBytes(t, model, "original weights")
	originalModTime := modTime(t, model)

	first := snapshotModelDigest(t, configPath, hashes)
	if first != sha256Hex("original weights") {
		t.Fatalf("first snapshot must hash the file content: %s", first)
	}

	writeModelBytes(t, model, "tampered weights")
	setModTime(t, model, originalModTime)
	if unchanged := snapshotModelDigest(t, configPath, hashes); unchanged != first {
		t.Fatalf("unchanged size and mtime must reuse the known hash instead of reading the file: %s != %s", unchanged, first)
	}

	setModTime(t, model, originalModTime.Add(time.Hour))
	if touched := snapshotModelDigest(t, configPath, hashes); touched != sha256Hex("tampered weights") {
		t.Fatalf("mtime change must re-hash the file: %s", touched)
	}

	writeModelBytes(t, model, "resized model weights")
	setModTime(t, model, originalModTime.Add(time.Hour))
	if resized := snapshotModelDigest(t, configPath, hashes); resized != sha256Hex("resized model weights") {
		t.Fatalf("size change must re-hash the file: %s", resized)
	}
}

func snapshotModelDigest(t *testing.T, configPath string, hashes *catalog.Catalog) string {
	t.Helper()
	snapshot, err := loadcapture.BuildSnapshot(configPath, hashes.HashFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Assets) != 1 {
		t.Fatalf("expected one model asset: %#v", snapshot.Assets)
	}
	return snapshot.Assets[0].SHA256
}

func writeModelConfig(t *testing.T, dir string, model string) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{"model_param": model})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "model.kcpps")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func writeModelBytes(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func modTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

func setModTime(t *testing.T, path string, modified time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
