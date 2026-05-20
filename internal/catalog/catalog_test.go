package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListReturnsPublicIDsWithoutKcppsExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.kcpps"), 0o755); err != nil {
		t.Fatal(err)
	}

	models, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "a" || models[1].ID != "b" {
		t.Fatalf("unexpected model order %#v", models)
	}
	if models[0].Filename != "a.kcpps" || models[1].Filename != "b.kcpps" {
		t.Fatalf("unexpected filenames %#v", models)
	}
}

func TestResolveRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok, err := New(dir).Resolve("../a")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("path traversal should not match")
	}
}

func TestResolveMapsPublicIDToConfigFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	model, ok, err := New(dir).Resolve("a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected model")
	}
	if model.Filename != "a.kcpps" {
		t.Fatalf("unexpected filename %q", model.Filename)
	}
}

func TestMissingDirectoryReturnsEmptyCatalog(t *testing.T) {
	models, err := New(filepath.Join(t.TempDir(), "missing")).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("expected empty model list")
	}
}
