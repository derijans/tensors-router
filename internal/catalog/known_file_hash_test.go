package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFileUsesAKnownHashInsteadOfReadingTheWeights(t *testing.T) {
	catalog, err := NewWithStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(weights, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	const knownHash = "2222222222222222222222222222222222222222222222222222222222222222"
	catalog.UseKnownFileHashes(func(path string) (string, bool) {
		return knownHash, path == weights
	})

	if hash, ok := catalog.hashStore.HashFile(weights); !ok || hash != knownHash {
		t.Fatalf("weights were hashed again: hash=%q ok=%t", hash, ok)
	}
}
