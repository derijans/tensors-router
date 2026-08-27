package proxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/siteapi"
)

func TestConfigFileIdentityKeepsTheNameOnDisk(t *testing.T) {
	id, filename, err := configFileIdentity(siteapi.ConfigFileRequest{Filename: "CaseExt.KCPPS"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "CaseExt" {
		t.Fatalf("id %q lost the case of the file it names", id)
	}
	if filename != "CaseExt.KCPPS" {
		t.Fatalf("filename %q does not name the file on disk", filename)
	}
}

func TestConfigFileIdentityKeepsTheNameWhenTheIDAgrees(t *testing.T) {
	id, filename, err := configFileIdentity(siteapi.ConfigFileRequest{ID: "Krea2Turbo", Filename: "Krea2Turbo.kcpps"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "Krea2Turbo" || filename != "Krea2Turbo.kcpps" {
		t.Fatalf("identity changed case: id=%q filename=%q", id, filename)
	}
}

func TestConfigFileIdentityBuildsANameForANewID(t *testing.T) {
	id, filename, err := configFileIdentity(siteapi.ConfigFileRequest{ID: "renamed", Filename: "Krea2Turbo.kcpps"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "renamed" || filename != "renamed.kcpps" {
		t.Fatalf("rename did not target a new file: id=%q filename=%q", id, filename)
	}
}

func TestConfigFileIdentitySanitizesAnUnusableStem(t *testing.T) {
	id, filename, err := configFileIdentity(siteapi.ConfigFileRequest{Filename: "my model.kcpps"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "my-model" || filename != "my-model.kcpps" {
		t.Fatalf("unsanitized stem leaked through: id=%q filename=%q", id, filename)
	}
}

func TestLocalConfigFileTargetAcceptsAnUppercaseExtension(t *testing.T) {
	service := NewService(ServiceConfig{ConfigDir: t.TempDir()})
	target, err := service.localConfigFileTarget("CaseExt.KCPPS")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(target) != "CaseExt.KCPPS" {
		t.Fatalf("target %q does not name the file on disk", target)
	}
}

func TestLocalConfigFileTargetStillRejectsOtherExtensions(t *testing.T) {
	service := NewService(ServiceConfig{ConfigDir: t.TempDir()})
	for _, filename := range []string{"model.json", "model.kcpps.txt", "model"} {
		if _, err := service.localConfigFileTarget(filename); err == nil {
			t.Fatalf("expected %q to be rejected", filename)
		}
	}
}

// A config discovered as .KCPPS has to resolve its assets under that exact name;
// rebuilding it as .kcpps only finds the file on a case-insensitive filesystem.
func TestEnsureModelAssetsResolvesAConfigNamedWithAnUppercaseExtension(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "weights.gguf")
	if err := os.WriteFile(assetPath, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	asset, err := index.IndexFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "Portable.KCPPS")
	content := `{"model_param_hash":"` + asset.SHA256 + `","model_param_filename":"weights.gguf"}`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Catalog: catalog.New(root), ConfigDir: root, AssetIndex: index})

	if err := service.ensureModelAssets(context.Background(), "Portable.KCPPS"); err != nil {
		t.Fatal(err)
	}

	resolved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(resolved), "_hash") || !strings.Contains(string(resolved), "weights.gguf") {
		t.Fatalf("portable config was not resolved: %s", resolved)
	}
}
