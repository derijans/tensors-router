//go:build !windows

package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkipsUnreadableDirectoriesInsteadOfFailing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory, so permission denial cannot be reproduced")
	}
	root := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(root, "model.gguf"), []byte("weights"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "credstore")
	if err := os.Mkdir(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	files, err := Scan([]string{root}, nil, "node")
	if err != nil {
		t.Fatalf("one unreadable directory failed the whole inventory: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0].Path) != "model.gguf" {
		t.Fatalf("readable files were not listed: %#v", files)
	}
}
