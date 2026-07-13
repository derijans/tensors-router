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
  admin_keys:
    - "admin-alpha"

models:
  config_dir: "./models"
  startup_model: "alpha"
  file_roots:
    - "C:/models"
    - "D:/assets"

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
  backend_logs_to_disk: true

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

analytics:
  enabled: true
  vram_enabled: false
  flush_interval: "2m"
  database_path: "./store/custom-analytics.sqlite"
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
	if !reflect.DeepEqual(cfg.Auth.InferenceKeys, []string{"alpha", "beta"}) || !reflect.DeepEqual(cfg.Auth.AdminKeys, []string{"admin-alpha"}) {
		t.Fatalf("unexpected split auth %#v", cfg.Auth)
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
	if !reflect.DeepEqual(cfg.Models.FileRoots, []string{"C:/models", "D:/assets"}) {
		t.Fatalf("unexpected file roots %#v", cfg.Models.FileRoots)
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
	if cfg.Logging.Mode != LoggingModeQuiet || len(cfg.Warnings) != 2 {
		t.Fatalf("unexpected compatibility result mode=%q warnings=%#v", cfg.Logging.Mode, cfg.Warnings)
	}
	if !cfg.Logging.BackendLogsToDisk {
		t.Fatalf("backend logs to disk should be enabled")
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
	if !cfg.Analytics.Enabled || cfg.Analytics.VRAMEnabled || cfg.Analytics.FlushInterval != 2*time.Minute || cfg.Analytics.DatabasePath != "./store/custom-analytics.sqlite" {
		t.Fatalf("unexpected analytics config %#v", cfg.Analytics)
	}
}

func TestLoadRejectsMissingRouterConfig(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "config.yaml")); err == nil {
		t.Fatal("expected missing router config error")
	}
}

func TestLoadExampleConfigIncludesVRAMAnalyticsDefault(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Analytics.Enabled || !cfg.Analytics.VRAMEnabled || cfg.Analytics.FlushInterval != 3*time.Minute {
		t.Fatalf("unexpected example analytics config %#v", cfg.Analytics)
	}
}

func TestLoadAcceptsRepositoryUpdateSourceWithOptionalHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
updates:
  enabled: true
  binary_url: ""
  binary_sha256: ""
  binary_repository_url: "https://github.com/LostRuins/koboldcpp"
  binary_asset_glob: "*vulkan*"
  include_prereleases: true
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Updates.IncludePrereleases || cfg.Updates.KoboldSource().RepositoryURL != "https://github.com/LostRuins/koboldcpp" || cfg.Updates.KoboldSource().AssetGlob != "*vulkan*" {
		t.Fatalf("unexpected update source %#v", cfg.Updates)
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

func TestLoadRejectsInvalidAnalyticsInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
analytics:
  flush_interval: "0s"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected invalid analytics interval error")
	}
}

func TestLoadRejectsRemovedHostURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
kobold:
  host_url: "https://ui.example.test/kobold"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected removed host url key error")
	}
}

func TestLoadSecurityProfileOverrideHasPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
security:
  profile: "secure"
server:
  bind: "0.0.0.0:8080"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected secure non-loopback credentials error")
	}
	cfg, err := LoadWithOptions(path, LoadOptions{SecurityProfile: SecurityProfileTrustedLAN})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.Profile != SecurityProfileTrustedLAN {
		t.Fatalf("unexpected profile %q", cfg.Security.Profile)
	}
}

func TestLoadRejectsCredentialPlaceholder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
auth:
  inference_keys: ["change-me"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected placeholder rejection")
	}
}

func TestLoadRejectsNonLoopbackManagedBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
kobold:
  backend_url: "http://192.168.1.20:5001"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected backend loopback rejection")
	}
}

func TestLoadRejectsBackendBindOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
kobold:
  extra_args: ["--host=0.0.0.0"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected backend bind override rejection")
	}
}

func TestLoadRejectsUnlimitedTransportLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
limits:
  max_stream_request_gb: 0
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected zero limit rejection")
	}
}

func TestDefaultsIncludeSecureStreamingAndRetentionValues(t *testing.T) {
	cfg := Defaults()
	if cfg.Security.Profile != SecurityProfileSecure || cfg.Server.Bind != "127.0.0.1:8080" {
		t.Fatalf("unexpected secure defaults %#v", cfg)
	}
	if cfg.Limits.ReplayBufferMB != 64 || cfg.Limits.MemoryBudgetMB != 2048 || cfg.Limits.DrainTimeout != 15*time.Minute {
		t.Fatalf("unexpected limits %#v", cfg.Limits)
	}
	if cfg.Analytics.RawRetention != 30*24*time.Hour || cfg.Analytics.VRAMSampleInterval != time.Second {
		t.Fatalf("unexpected analytics defaults %#v", cfg.Analytics)
	}
}

func TestResolveSecurityProfilePrefersCLI(t *testing.T) {
	if got := ResolveSecurityProfile(SecurityProfileSecure, SecurityProfileTrustedLAN); got != SecurityProfileSecure {
		t.Fatalf("unexpected resolved profile %q", got)
	}
	if got := ResolveSecurityProfile("", SecurityProfileTrustedLAN); got != SecurityProfileTrustedLAN {
		t.Fatalf("unexpected environment profile %q", got)
	}
}

func TestContainerRouterExamplesAreValid(t *testing.T) {
	for _, name := range []string{"node.yaml", "router-managed.yaml"} {
		if _, err := Load(filepath.Join("..", "..", "deploy", "config", name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestLoadRejectsIncoherentOrOverflowingTransportLimits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	for _, content := range []string{
		"limits:\n  replay_buffer_mb: 65\n  memory_budget_mb: 64\n",
		"limits:\n  replay_buffer_mb: 1\n  memory_budget_mb: 33\n",
		"limits:\n  max_stream_request_gb: 9223372036854775807\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("expected limit validation error for %q", content)
		}
	}
}
