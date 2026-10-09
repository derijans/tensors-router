package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecureAssetDirectoryAcceptsASymlinkedSharedRoot(t *testing.T) {
	realRoot := t.TempDir()
	linkedRoot := filepath.Join(t.TempDir(), "model-assets")
	symlinkOrSkip(t, realRoot, linkedRoot)
	target := filepath.Join(linkedRoot, "sha256", "ab", "abcdef")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	if !secureAssetDirectory(linkedRoot, target) {
		t.Fatal("a shared asset root on another disk via symlink must stay usable for peer promotion")
	}
}

func TestSecureAssetDirectoryRejectsASymlinkBelowTheSharedRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sha256"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(root, "sha256", "ab"))

	if secureAssetDirectory(root, filepath.Join(root, "sha256", "ab")) {
		t.Fatal("a symlinked directory below the shared root must be rejected")
	}
}

func TestOpenRegularPartialFileDoesNotCreateASymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	partial := filepath.Join(dir, "asset.partial")
	symlinkOrSkip(t, victim, partial)

	if file, ok := openRegularPartialFile(partial); ok {
		file.Close()
		t.Fatal("a symlinked partial file must be rejected")
	}
	if _, err := os.Lstat(victim); !os.IsNotExist(err) {
		t.Fatalf("opening the partial file created the symlink target: %v", err)
	}
}

func symlinkOrSkip(t *testing.T, target string, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}
