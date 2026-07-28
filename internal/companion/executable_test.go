package companion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSiblingUsesReleaseSuffix(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "tensor-router-downloader-linux-amd64")
	if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, found := FindSibling(filepath.Join(directory, "tensors-router-linux-amd64"), "tensor-router-downloader", "tensors-router")
	if !found || path != target {
		t.Fatalf("release companion was not found path=%q found=%v", path, found)
	}
}

func TestFindSiblingFallsBackToUnsuffixedName(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "tensor-router-downloader")
	if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, found := FindSibling(filepath.Join(directory, "tensors-router-linux-amd64"), "tensor-router-downloader", "tensors-router")
	if !found || path != target {
		t.Fatalf("unsuffixed companion was not found path=%q found=%v", path, found)
	}
}
