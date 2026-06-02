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

cluster:
  role: "master"
  node_id: "master-a"
  public_url: "http://127.0.0.1:8080"
  master_url: ""
  slave_urls:
    - "http://127.0.0.1:8081"
  token: "cluster-secret"
  store_dir: "./store"
  sync_interval: "30s"
  health_interval: "5s"
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
	if cfg.Cluster.Role != "master" || cfg.Cluster.NodeID != "master-a" {
		t.Fatalf("unexpected cluster identity %#v", cfg.Cluster)
	}
	if !reflect.DeepEqual(cfg.Cluster.SlaveURLs, []string{"http://127.0.0.1:8081"}) {
		t.Fatalf("unexpected slave urls %#v", cfg.Cluster.SlaveURLs)
	}
	if cfg.Cluster.Token != "cluster-secret" || cfg.Cluster.StoreDir != "./store" {
		t.Fatalf("unexpected cluster config %#v", cfg.Cluster)
	}
	if cfg.Cluster.SyncInterval != 30*time.Second || cfg.Cluster.HealthInterval != 5*time.Second {
		t.Fatalf("unexpected cluster intervals %#v", cfg.Cluster)
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
	if cfg.Cluster.Role != "standalone" || cfg.Cluster.NodeID != "local" {
		t.Fatalf("unexpected default cluster %#v", cfg.Cluster)
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

func TestLoadRejectsSlaveClusterWithoutRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
cluster:
  role: "slave"
  node_id: "slave-a"
  token: "secret"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected missing slave fields error")
	}
}
