package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRouterAppliesRequestDefaultsAndSharesRuntimesByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("models:\n  config_dir: ./kcpps\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Models.InjectRequestDefaults || !cfg.Models.ShareRuntimeByLaunchArguments {
		t.Fatalf("models = %+v, want both request-profile settings on", cfg.Models)
	}
}

func TestRequestProfileSettingsCanBeSwitchedOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "models:\n  inject_request_defaults: false\n  share_runtime_by_launch_arguments: false\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Models.InjectRequestDefaults || cfg.Models.ShareRuntimeByLaunchArguments {
		t.Fatalf("models = %+v, want both request-profile settings off", cfg.Models)
	}
}
