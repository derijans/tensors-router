package credential

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveValueOrFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, present, err := Resolve(Source{Role: "admin", FileName: "TOKEN_FILE", FilePath: path})
	if err != nil || !present || value != "file-secret" {
		t.Fatalf("unexpected file result value=%q present=%t err=%v", value, present, err)
	}
	value, present, err = Resolve(Source{Role: "admin", ValueName: "TOKEN", Value: " direct-secret "})
	if err != nil || !present || value != "direct-secret" {
		t.Fatalf("unexpected value result value=%q present=%t err=%v", value, present, err)
	}
}

func TestResolveRejectsAmbiguousOrInvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Resolve(Source{Role: "admin", ValueName: "TOKEN", Value: "secret", FileName: "TOKEN_FILE", FilePath: path})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected ambiguity rejection, got %v", err)
	}
	if _, _, err := Resolve(Source{Role: "admin", FileName: "TOKEN_FILE", FilePath: dir}); err == nil {
		t.Fatal("expected non-regular file rejection")
	}
	if err := os.WriteFile(path, []byte("first\nsecond"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(Source{Role: "admin", FileName: "TOKEN_FILE", FilePath: path}); err == nil {
		t.Fatal("expected multiline credential rejection")
	}
}

func TestResolveRejectsMultilineDirectCredential(t *testing.T) {
	_, _, err := Resolve(Source{Role: "admin", ValueName: "TOKEN", Value: "secret\n"})
	if err == nil || !strings.Contains(err.Error(), "one credential") {
		t.Fatalf("expected direct multiline credential rejection, got %v", err)
	}
}

func TestResolveRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target")
	linkPath := filepath.Join(dir, "link")
	if err := os.WriteFile(targetPath, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, linkPath); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symbolic link creation is unavailable: %v", err)
		}
		t.Fatal(err)
	}
	_, _, err := Resolve(Source{Role: "admin", FileName: "TOKEN_FILE", FilePath: linkPath})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symbolic link rejection, got %v", err)
	}
}
