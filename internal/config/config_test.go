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

backend:
  mode: "llama_sdcpp"

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

llama:
  backend_url: "http://127.0.0.1:6002"
  binary_path: "./bin/llama-server"
  data_dir: "./llama-state"
  hide_window: false
  extra_args:
    - "--parallel"
    - "2"

sdcpp:
  backend_url: "http://127.0.0.1:7861"
  binary_path: "./bin/sd-server"
  data_dir: "./sd-state"
  hide_window: false
  extra_args: ["--verbose"]

logging:
  enabled: false

updates:
  enabled: false
  check_interval: "24h"
  binary_url: "https://example.test/koboldcpp"
  binary_sha256: "0000000000000000000000000000000000000000000000000000000000000001"
  llama_binary_url: "https://example.test/llama-server"
  llama_binary_sha256: "0000000000000000000000000000000000000000000000000000000000000002"
  sdcpp_binary_url: "https://example.test/sd-server"
  sdcpp_binary_sha256: "0000000000000000000000000000000000000000000000000000000000000003"

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
	if cfg.Backend.Mode != "llama_sdcpp" {
		t.Fatalf("unexpected backend mode %q", cfg.Backend.Mode)
	}
	if cfg.Kobold.Quiet || cfg.Kobold.SkipLauncher || cfg.Kobold.NoModel || cfg.Kobold.HideWindow {
		t.Fatalf("unexpected kobold bool settings %#v", cfg.Kobold)
	}
	if cfg.Llama.BackendURL != "http://127.0.0.1:6002" || cfg.Llama.BinaryPath != "./bin/llama-server" || cfg.Llama.DataDir != "./llama-state" || cfg.Llama.HideWindow {
		t.Fatalf("unexpected llama config %#v", cfg.Llama)
	}
	if !reflect.DeepEqual(cfg.Llama.ExtraArgs, []string{"--parallel", "2"}) {
		t.Fatalf("unexpected llama extra args %#v", cfg.Llama.ExtraArgs)
	}
	if cfg.SDCPP.BackendURL != "http://127.0.0.1:7861" || cfg.SDCPP.BinaryPath != "./bin/sd-server" || cfg.SDCPP.DataDir != "./sd-state" || cfg.SDCPP.HideWindow {
		t.Fatalf("unexpected sdcpp config %#v", cfg.SDCPP)
	}
	if !reflect.DeepEqual(cfg.SDCPP.ExtraArgs, []string{"--verbose"}) {
		t.Fatalf("unexpected sdcpp extra args %#v", cfg.SDCPP.ExtraArgs)
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
	if cfg.Updates.BinaryURL != "https://example.test/koboldcpp" || cfg.Updates.LlamaBinaryURL != "https://example.test/llama-server" || cfg.Updates.SDCPPBinaryURL != "https://example.test/sd-server" {
		t.Fatalf("unexpected update urls %#v", cfg.Updates)
	}
	if cfg.Updates.BinarySHA256 != "0000000000000000000000000000000000000000000000000000000000000001" || cfg.Updates.LlamaSHA256 != "0000000000000000000000000000000000000000000000000000000000000002" || cfg.Updates.SDCPPSHA256 != "0000000000000000000000000000000000000000000000000000000000000003" {
		t.Fatalf("unexpected update hashes %#v", cfg.Updates)
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
	if cfg.Kobold.BinaryPath != "./bin/kobold/koboldcpp" {
		t.Fatalf("unexpected binary path %q", cfg.Kobold.BinaryPath)
	}
	if cfg.Models.StartupModel != "" {
		t.Fatalf("unexpected startup model %q", cfg.Models.StartupModel)
	}
	if cfg.Backend.Mode != "kobold" {
		t.Fatalf("unexpected default backend mode %q", cfg.Backend.Mode)
	}
	if !cfg.Kobold.Quiet || !cfg.Kobold.SkipLauncher || !cfg.Kobold.NoModel || !cfg.Kobold.HideWindow {
		t.Fatalf("default kobold bool settings should be enabled")
	}
	if cfg.Llama.BackendURL != "http://127.0.0.1:5002" || cfg.SDCPP.BackendURL != "http://127.0.0.1:7860" {
		t.Fatalf("unexpected native defaults llama=%#v sdcpp=%#v", cfg.Llama, cfg.SDCPP)
	}
	if cfg.Llama.BinaryPath != "./bin/llama/llama-b9495/llama-server" || cfg.SDCPP.BinaryPath != "./bin/stable-diffusion/build/bin/sd-server" {
		t.Fatalf("unexpected native binary defaults llama=%q sdcpp=%q", cfg.Llama.BinaryPath, cfg.SDCPP.BinaryPath)
	}
	if !cfg.Logging.Enabled {
		t.Fatalf("default logging should be enabled")
	}
	if cfg.Updates.Enabled {
		t.Fatalf("default updates should be disabled until checksum pins are configured")
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

func TestLoadRejectsEnabledUpdateWithoutHTTPSAndSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
updates:
  enabled: true
  binary_url: "http://example.test/koboldcpp"
  binary_sha256: "not-a-hash"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected insecure update config error")
	}
}

func TestLoadAcceptsEnabledSplitUpdatesWithSHA256Pins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
backend:
  mode: "llama_sdcpp"

updates:
  enabled: true
  llama_binary_url: "https://example.test/llama-server"
  llama_binary_sha256: "0000000000000000000000000000000000000000000000000000000000000001"
  sdcpp_binary_url: "https://example.test/sd-server"
  sdcpp_binary_sha256: "0000000000000000000000000000000000000000000000000000000000000002"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err != nil {
		t.Fatalf("expected valid split update config: %v", err)
	}
}
