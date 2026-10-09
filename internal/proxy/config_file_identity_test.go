package proxy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cook"
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

func TestConfigFileIdentityKeepsADottedNameOnDisk(t *testing.T) {
	id, filename, err := configFileIdentity(siteapi.ConfigFileRequest{Filename: "qwen3.8-27b-instruct.kcpps"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "qwen3.8-27b-instruct" || filename != "qwen3.8-27b-instruct.kcpps" {
		t.Fatalf("dotted config renamed to a file that does not exist: id=%q filename=%q", id, filename)
	}
}

func TestConfigFileIdentityStillSanitizesDotOnlyTricks(t *testing.T) {
	for _, requested := range []string{".hidden.kcpps", "a..b.kcpps"} {
		_, filename, err := configFileIdentity(siteapi.ConfigFileRequest{Filename: requested})
		if err == nil && filename == requested {
			t.Fatalf("%q was taken as a name on disk", requested)
		}
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

	if err := service.assets.ensure(context.Background(), "Portable.KCPPS"); err != nil {
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

func TestSaveConfigByIDRewritesTheFileWithAnUppercaseExtension(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "CaseExt.KCPPS"), []byte(`{"threads":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{ConfigDir: configDir})

	response, err := service.saveLocalConfigFile(siteapi.ConfigFileRequest{ID: "CaseExt", Overwrite: true, Options: cook.Options{"threads": json.RawMessage("2")}}, false)
	if err != nil {
		t.Fatal(err)
	}

	if response.Filename != "CaseExt.KCPPS" {
		t.Fatalf("save by id targeted %q instead of the config on disk", response.Filename)
	}
	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "CaseExt.KCPPS" {
		t.Fatalf("save by id created a second config with the same id: %v", entries)
	}
}

func TestDeleteConfigByIDRemovesTheFileWithAnUppercaseExtension(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "CaseExt.KCPPS"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{ConfigDir: configDir})

	response, err := service.deleteLocalConfigFile(siteapi.ConfigFileRequest{ID: "CaseExt"})
	if err != nil {
		t.Fatal(err)
	}

	if response.Filename != "CaseExt.KCPPS" {
		t.Fatalf("delete by id targeted %q instead of the config on disk", response.Filename)
	}
	if _, err := os.Stat(filepath.Join(configDir, "CaseExt.KCPPS")); !os.IsNotExist(err) {
		t.Fatalf("config with an uppercase extension survived delete by id: %v", err)
	}
}
