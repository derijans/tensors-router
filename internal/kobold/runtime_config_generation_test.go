package kobold

import (
	"os"
	"path/filepath"
	"testing"

	"tensors-router/internal/catalog"
)

const textConfigWithSamplerDefaults = `{"model_param":"/models/text.gguf","contextsize":8192,"gendefaults":"{\"temperature\":0.2}","gendefaultsoverwrite":true}`

func managerForRuntimeConfig(t *testing.T, config string, inject bool) *Manager {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "variant.kcpps"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := catalog.RequestProfilePolicy{InjectGenerationDefaults: inject, DefaultBackendMode: "kobold"}
	manager, err := NewManager(ProcessConfig{BackendURL: "http://127.0.0.1:5001", ConfigDir: dir, Multiuser: 1, RequestProfiles: policy})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestRuntimeConfigLeavesSamplerDefaultsToTheRouterWhenItInjectsThem(t *testing.T) {
	manager := managerForRuntimeConfig(t, textConfigWithSamplerDefaults, true)

	runtimeFilename, generatedPath, _, err := manager.runtimeConfig("variant.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Dir(runtimeFilename) != routerRuntimeDirectory {
		t.Fatalf("runtime filename = %q, want a generated config", runtimeFilename)
	}
	values := readRuntimeConfigValues(t, generatedPath)
	for _, stripped := range generationDefaultsKeys {
		if _, present := values[stripped]; present {
			t.Fatalf("generated config kept %s, so a stale variant could supply its defaults: %v", stripped, values)
		}
	}
	if string(values["contextsize"]) != "8192" || string(values["model_param"]) != `"/models/text.gguf"` {
		t.Fatalf("generated config lost launch settings: %v", values)
	}
}

func TestRuntimeConfigKeepsSamplerDefaultsWhenTheBackendAppliesThem(t *testing.T) {
	manager := managerForRuntimeConfig(t, textConfigWithSamplerDefaults, false)

	runtimeFilename, generatedPath, _, err := manager.runtimeConfig("variant.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	if runtimeFilename != "variant.kcpps" || generatedPath != "" {
		t.Fatalf("runtime config = %q %q, want the original file", runtimeFilename, generatedPath)
	}
}

func TestRuntimeConfigWithoutSamplerDefaultsLoadsTheOriginalFile(t *testing.T) {
	for name, config := range map[string]string{
		"absent":    `{"model_param":"/models/text.gguf"}`,
		"empty":     `{"model_param":"/models/text.gguf","gendefaults":"","gendefaultsoverwrite":false}`,
		"malformed": `{"model_param":"/models/text.gguf","gendefaults":"temperature:0.35"}`,
	} {
		manager := managerForRuntimeConfig(t, config, true)

		runtimeFilename, generatedPath, _, err := manager.runtimeConfig("variant.kcpps")
		if err != nil {
			t.Fatal(err)
		}

		if runtimeFilename != "variant.kcpps" || generatedPath != "" {
			t.Fatalf("%s: runtime config = %q %q, want the original file", name, runtimeFilename, generatedPath)
		}
	}
}

func TestRuntimeConfigKeepsSamplerDefaultsForImageConfigs(t *testing.T) {
	manager := managerForRuntimeConfig(t, `{"model_param":"/models/text.gguf","sdmodel":"/models/image.safetensors","gendefaults":{"steps":20}}`, true)

	runtimeFilename, _, _, err := manager.runtimeConfig("variant.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	if runtimeFilename != "variant.kcpps" {
		t.Fatalf("runtime filename = %q, want the original file so KoboldCpp keeps its image defaults", runtimeFilename)
	}
}
