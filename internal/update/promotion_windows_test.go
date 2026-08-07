//go:build windows

package update

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLockedArchiveExecutableKeepsIncumbent(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "installed")
	stagingDir := filepath.Join(root, "staging")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldContent := bytes.Repeat([]byte("o"), 256*1024)
	newContent := bytes.Repeat([]byte("n"), 256*1024)
	targetBinary := filepath.Join(targetDir, "server.exe")
	if err := os.WriteFile(targetBinary, oldContent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "server.exe"), newContent, 0o755); err != nil {
		t.Fatal(err)
	}
	locked, err := os.Open(targetBinary)
	if err != nil {
		t.Fatal(err)
	}
	promotion, promotionErr := promoteArchiveTree(stagingDir, targetDir, "server.exe")
	if promotionErr == nil {
		promotionErr = promotion.Commit()
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if promotionErr == nil {
		t.Fatal("expected locked executable replacement to fail")
	}
	content, err := os.ReadFile(targetBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, oldContent) {
		t.Fatal("locked archive replacement changed the incumbent executable")
	}
}
