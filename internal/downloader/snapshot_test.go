package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadedSnapshotDigestIsStableAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	repository := "owner/model"
	commit := "0123456789abcdef"
	files := []JobFile{{Path: "weights/a.bin", Size: 1}, {Path: "config.json", Size: 2}}
	for _, file := range files {
		destination, err := snapshotDestinationPath(root, repository, commit, file.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, make([]byte, file.Size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := computeDownloadedSnapshotDigest(root, repository, commit, files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := computeDownloadedSnapshotDigest(root, repository, commit, []JobFile{files[1], files[0]})
	if err != nil || first != second || len(first) != 64 {
		t.Fatalf("snapshot digest was unstable first=%q second=%q error=%v", first, second, err)
	}
	snapshotRoot, err := SnapshotDirectory(root, repository, commit)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(snapshotRoot, "config.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(snapshotRoot, "weights", "a.bin"), filepath.Join(snapshotRoot, "config.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := computeDownloadedSnapshotDigest(root, repository, commit, files); err == nil {
		t.Fatal("snapshot symlink was accepted")
	}
}
