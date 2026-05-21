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

func TestImageModelsUseVirtualIDsAndActiveCombinedConfig(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "llm.kcpps", `{}`)
	writeCatalogFile(t, dir, "image.kcpps", `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`)
	writeCatalogFile(t, dir, "combo.kcpps", `{"model_param":"C:\\models\\llm.gguf","sdmodel":"/models/vision.ckpt"}`)

	models, err := New(dir).ListLLM()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "combo" || models[1].ID != "llm" {
		t.Fatalf("unexpected llm models %#v", models)
	}

	images, err := New(dir).ListImages("")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ImageID != "image-dream" {
		t.Fatalf("unexpected inactive images %#v", images)
	}

	images, err = New(dir).ListImages("combo.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0].ImageID != "combo-vision" || images[1].ImageID != "image-dream" {
		t.Fatalf("unexpected active images %#v", images)
	}

	_, ok, err := New(dir).ResolveImage("combo-vision", "")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("inactive combined image should not resolve")
	}

	model, ok, err := New(dir).ResolveImage("combo-vision", "combo.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || model.Filename != "combo.kcpps" {
		t.Fatalf("unexpected active combined resolve %#v %v", model, ok)
	}
}

func writeCatalogFile(t *testing.T, dir string, filename string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
