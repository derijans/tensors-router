package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadYAMLOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
server:
  bind: "127.0.0.1:9999"
  allowed_cidrs:
    - "127.0.0.0/8"

auth:
  bearer_keys:
    - "alpha"
    - "beta"

models:
  config_dir: "./models"
  startup_model: "alpha"

kobold:
  backend_url: "http://127.0.0.1:6000"
  binary_path: "./bin/koboldcpp"
  data_dir: "./state"
  multiuser: 2
  quiet: false
  skip_launcher: false
  no_model: false
  hide_window: false
  extra_args: ["--flashattention", "--quiet"]

logging:
  enabled: false

updates:
  enabled: false
  check_interval: "24h"
  binary_url: "https://example.test/koboldcpp"
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Bind != "127.0.0.1:9999" {
		t.Fatalf("unexpected bind %q", cfg.Server.Bind)
	}
	if !reflect.DeepEqual(cfg.Auth.BearerKeys, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected bearer keys %#v", cfg.Auth.BearerKeys)
	}
	if !reflect.DeepEqual(cfg.Kobold.ExtraArgs, []string{"--flashattention", "--quiet"}) {
		t.Fatalf("unexpected extra args %#v", cfg.Kobold.ExtraArgs)
	}
	if cfg.Kobold.Multiuser != 2 {
		t.Fatalf("unexpected multiuser %d", cfg.Kobold.Multiuser)
	}
	if cfg.Models.StartupModel != "alpha" {
		t.Fatalf("unexpected startup model %q", cfg.Models.StartupModel)
	}
	if cfg.Kobold.Quiet || cfg.Kobold.SkipLauncher || cfg.Kobold.NoModel || cfg.Kobold.HideWindow {
		t.Fatalf("unexpected kobold bool settings %#v", cfg.Kobold)
	}
	if cfg.Logging.Enabled {
		t.Fatalf("logging should be disabled")
	}
	if cfg.Updates.Enabled {
		t.Fatalf("updates should be disabled")
	}
	if cfg.Updates.CheckInterval != 24*time.Hour {
		t.Fatalf("unexpected check interval %s", cfg.Updates.CheckInterval)
	}
}

func TestLoadDefaultConfigWhenDefaultFileMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kobold.BinaryPath != "./bin/koboldcpp" {
		t.Fatalf("unexpected binary path %q", cfg.Kobold.BinaryPath)
	}
	if cfg.Models.StartupModel != "" {
		t.Fatalf("unexpected startup model %q", cfg.Models.StartupModel)
	}
	if !cfg.Kobold.Quiet || !cfg.Kobold.SkipLauncher || !cfg.Kobold.NoModel || !cfg.Kobold.HideWindow {
		t.Fatalf("default kobold bool settings should be enabled")
	}
	if !cfg.Logging.Enabled {
		t.Fatalf("default logging should be enabled")
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  nope: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected unknown key error")
	}
}
