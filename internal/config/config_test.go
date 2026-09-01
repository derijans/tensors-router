package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
  embeddings_backend_url: "http://127.0.0.1:6004"
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
  embeddings_backend_url: "http://127.0.0.1:6005"
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

whispercpp:
  backend_url: "http://127.0.0.1:6003"
  binary_path: "./bin/whisper-server"
  data_dir: "./whisper-state"
  hide_window: false
  extra_args: ["--language", "auto"]

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
  whispercpp_binary_url: "https://example.test/whisper-server"
  whispercpp_binary_sha256: "0000000000000000000000000000000000000000000000000000000000000004"
  whispercpp_repository_url: "https://github.com/ggml-org/whisper.cpp"
  whispercpp_asset_glob: "whisper-bin-x64.zip"

downloader:
  enabled: false
  binary_location: "./tools/tensor-router-downloader"

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
	if cfg.Kobold.EmbeddingsBackendURL != "http://127.0.0.1:6004" {
		t.Fatalf("unexpected kobold embeddings URL %q", cfg.Kobold.EmbeddingsBackendURL)
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
	if cfg.Llama.EmbeddingsBackendURL != "http://127.0.0.1:6005" {
		t.Fatalf("unexpected llama embeddings URL %q", cfg.Llama.EmbeddingsBackendURL)
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
	if cfg.WhisperCPP.BackendURL != "http://127.0.0.1:6003" || cfg.WhisperCPP.BinaryPath != "./bin/whisper-server" || cfg.WhisperCPP.DataDir != "./whisper-state" || cfg.WhisperCPP.HideWindow {
		t.Fatalf("unexpected whispercpp config %#v", cfg.WhisperCPP)
	}
	if !reflect.DeepEqual(cfg.WhisperCPP.ExtraArgs, []string{"--language", "auto"}) {
		t.Fatalf("unexpected whispercpp extra args %#v", cfg.WhisperCPP.ExtraArgs)
	}
	if cfg.Logging.Enabled {
		t.Fatalf("logging should be disabled")
	}
	if cfg.Logging.Mode != LoggingModeQuiet || len(cfg.Warnings) != 4 {
		t.Fatalf("unexpected compatibility result mode=%q warnings=%#v", cfg.Logging.Mode, cfg.Warnings)
	}
	if !anyContains(cfg.Warnings, "kobold.embeddings_backend_url is deprecated") || !anyContains(cfg.Warnings, "llama.embeddings_backend_url is deprecated") {
		t.Fatalf("expected embeddings_backend_url deprecation warnings, got %#v", cfg.Warnings)
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
	if cfg.Updates.BinaryURL != "https://example.test/koboldcpp" || cfg.Updates.LlamaBinaryURL != "https://example.test/llama-server" || cfg.Updates.SDCPPBinaryURL != "https://example.test/sd-server" || cfg.Updates.WhisperCPPBinaryURL != "https://example.test/whisper-server" {
		t.Fatalf("unexpected update urls %#v", cfg.Updates)
	}
	if cfg.Updates.BinarySHA256 != "0000000000000000000000000000000000000000000000000000000000000001" || cfg.Updates.LlamaSHA256 != "0000000000000000000000000000000000000000000000000000000000000002" || cfg.Updates.SDCPPSHA256 != "0000000000000000000000000000000000000000000000000000000000000003" || cfg.Updates.WhisperCPPSHA256 != "0000000000000000000000000000000000000000000000000000000000000004" {
		t.Fatalf("unexpected update hashes %#v", cfg.Updates)
	}
	if cfg.Updates.WhisperCPPRepositoryURL != "https://github.com/ggml-org/whisper.cpp" || cfg.Updates.WhisperCPPAssetGlob != "whisper-bin-x64.zip" {
		t.Fatalf("unexpected whispercpp update source %#v", cfg.Updates.WhisperCPPSource())
	}
	if cfg.Downloader.Enabled || cfg.Downloader.BinaryLocation != "./tools/tensor-router-downloader" {
		t.Fatalf("unexpected downloader config %#v", cfg.Downloader)
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

func TestValidateRejectsDuplicateBackendEndpoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
llama:
  backend_url: "http://127.0.0.1:5001"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected a collision between kobold.backend_url and llama.backend_url to be rejected")
	}
	if !strings.Contains(err.Error(), "kobold.backend_url") || !strings.Contains(err.Error(), "llama.backend_url") {
		t.Fatalf("expected the error to name both offending keys, got: %v", err)
	}
}

func TestValidateRejectsPortlessBackendURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
kobold:
  backend_url: "http://127.0.0.1"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected a portless backend URL to be rejected")
	}
	if !strings.Contains(err.Error(), "kobold.backend_url") || !strings.Contains(err.Error(), "explicit port") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAllowsDynamicEndpointsToShareTheZeroPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
kobold:
  embeddings_backend_url: "http://127.0.0.1:0"
llama:
  embeddings_backend_url: "http://127.0.0.1:0"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("expected two dynamic (port 0) endpoints not to collide, got: %v", err)
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
	if !cfg.Downloader.Enabled || cfg.Downloader.BinaryLocation != "" {
		t.Fatalf("unexpected downloader defaults %#v", cfg.Downloader)
	}
	// Port 0 means the router allocates a free loopback port at spawn
	// time, so the default embeddings endpoints can never collide with
	// each other or with the primary backend.
	if cfg.Kobold.EmbeddingsBackendURL != "http://127.0.0.1:0" || cfg.Llama.EmbeddingsBackendURL != "http://127.0.0.1:0" {
		t.Fatalf("unexpected embeddings endpoint defaults kobold=%q llama=%q", cfg.Kobold.EmbeddingsBackendURL, cfg.Llama.EmbeddingsBackendURL)
	}
}

func TestLoadRejectsNonLoopbackEmbeddingsBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("kobold:\n  embeddings_backend_url: http://192.168.1.2:5004\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "kobold.embeddings_backend_url") {
		t.Fatalf("expected loopback validation error, got %v", err)
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
	options := LoadOptions{InferenceKey: "deployment-inference", AdminKey: "deployment-admin"}
	for _, name := range []string{"node.yaml", "router-managed.yaml"} {
		if _, err := LoadWithOptions(filepath.Join("..", "..", "deploy", "config", name), options); err != nil {
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

func TestLoadCredentialOverridesReplaceConfiguredRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "security:\n  profile: \"secure\"\nserver:\n  bind: \"0.0.0.0:8080\"\nauth:\n  inference_keys: [\"configured-inference\"]\n  admin_keys: [\"configured-admin\"]\ncluster:\n  token: \"configured-cluster\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithOptions(path, LoadOptions{
		InferenceKey: "environment-inference",
		AdminKey:     "environment-admin",
		ClusterToken: "environment-cluster",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Auth.InferenceKeys, []string{"environment-inference"}) ||
		!reflect.DeepEqual(cfg.Auth.AdminKeys, []string{"environment-admin"}) ||
		cfg.Cluster.Token != "environment-cluster" {
		t.Fatalf("unexpected credential overrides %#v cluster=%q", cfg.Auth, cfg.Cluster.Token)
	}
}

func TestLoadCredentialOverrideReplacesLegacyBearerKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "auth:\n  bearer_keys: [legacy-inference]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithOptions(path, LoadOptions{InferenceKey: "environment-inference"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Auth.InferenceKeys, []string{"environment-inference"}) {
		t.Fatalf("unexpected inference credentials %#v", cfg.Auth.InferenceKeys)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("expected legacy configuration warning, got %#v", cfg.Warnings)
	}
}

func TestLoadRejectsCredentialReuseAcrossRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "auth:\n  inference_keys: [\"shared-secret\"]\n  admin_keys: [\"shared-secret\"]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected cross-role credential reuse rejection")
	}
}

func TestValidateRepositoryUpdatesRequireCanonicalTUFMetadataURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "missing", url: ""},
		{name: "http", url: "http://updates.example.test/metadata"},
		{name: "wrong path", url: "https://updates.example.test/tuf"},
		{name: "credentials", url: "https://token@updates.example.test/metadata"},
		{name: "query", url: "https://updates.example.test/metadata?channel=stable"},
		{name: "fragment", url: "https://updates.example.test/metadata#stable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Updates.Enabled = true
			cfg.Updates.TUFRepositoryURL = test.url
			if err := validate(&cfg); err == nil {
				t.Fatal("expected invalid TUF repository URL rejection")
			}
		})
	}
}

func TestValidateAcceptsBuiltInTrustedRepositoryUpdate(t *testing.T) {
	cfg := Defaults()
	cfg.Updates.Enabled = true
	cfg.Updates.TUFRepositoryURL += "/"
	if err := validate(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestLoadVLLMConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "backend:\n  mode: vllm\nvllm:\n  binary_location: ./tensor-router-vllm\n  data_dir: ./data/vllm\n  profile: cuda-12.9\n  manifest_path: ./profiles.json\n  tuf_repository_url: https://updates.example.test/metadata\n  tuf_root_path: ./root.json\n  dynamic_lora_enabled: true\n  eep_enabled: true\n  trust_remote_code: true\n  external_tools: true\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend.Mode != "vllm" || cfg.VLLM.Profile != "cuda-12.9" || !cfg.VLLM.DynamicLoRAEnabled || !cfg.VLLM.EEPEnabled || !cfg.VLLM.TrustRemoteCode || !cfg.VLLM.ExternalTools {
		t.Fatalf("unexpected vLLM configuration %#v", cfg.VLLM)
	}
}

func TestLoadStripsInlineCommentsWithoutTouchingQuotedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := strings.Join([]string{
		"server:  # trailing comment on a section header",
		`  bind: "127.0.0.1:9999" # trailing comment after a quoted value`,
		"  allowed_cidrs:   # comment on a list key",
		`    - "127.0.0.0/8" # comment on a list item`,
		"backend:",
		"  mode: vllm",
		"vllm:",
		"  data_dir: ./data/vllm",
		"  allow_unverified_install: true   # explicit opt-in",
		`  unverified_extra_index_url: "https://download.pytorch.org/whl/cu129" #line 61`,
		`  unverified_python_version: "3.12"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Bind != "127.0.0.1:9999" {
		t.Fatalf("unexpected bind %q", cfg.Server.Bind)
	}
	if !reflect.DeepEqual(cfg.Server.AllowedCIDRs, []string{"127.0.0.0/8"}) {
		t.Fatalf("unexpected allowed CIDRs %#v", cfg.Server.AllowedCIDRs)
	}
	if !cfg.VLLM.AllowUnverifiedInstall {
		t.Fatal("allow_unverified_install was not parsed with a trailing comment")
	}
	if cfg.VLLM.UnverifiedExtraIndexURL != "https://download.pytorch.org/whl/cu129" {
		t.Fatalf("unexpected extra index URL %q", cfg.VLLM.UnverifiedExtraIndexURL)
	}
}

func TestStripInlineCommentKeepsHashesThatAreNotComments(t *testing.T) {
	for input, expected := range map[string]string{
		`"https://example.test/whl/cu129" #line 61`: `"https://example.test/whl/cu129"`,
		`"a # b"`:                     `"a # b"`,
		`'a # b'`:                     `'a # b'`,
		`https://example.test/x#frag`: `https://example.test/x#frag`,
		`plain value`:                 `plain value`,
		`value\t# tabbed comment`:     `value\t# tabbed comment`,
		`""  # empty quoted`:          `""`,
		`# whole value is a comment`:  ``,
	} {
		if actual := stripInlineComment(input); actual != expected {
			t.Fatalf("stripInlineComment(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestValidateRejectsUnsafeVLLMProfile(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.Profile = "../../escape"
	if err := validate(&cfg); err == nil {
		t.Fatal("expected unsafe vLLM profile rejection")
	}
}

func TestValidateAcceptsOperatorPinnedVLLMManifest(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.TUFRepositoryURL = ""
	cfg.VLLM.ManifestPath = "vllm-manifest.json"
	cfg.VLLM.ManifestSHA256 = strings.Repeat("a", 64)
	cfg.VLLM.ManifestSize = 1024
	if err := validate(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsUnpinnedVLLMManifestWithoutTUF(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"no pin":       func(cfg *Config) {},
		"short digest": func(cfg *Config) { cfg.VLLM.ManifestSHA256 = strings.Repeat("a", 63); cfg.VLLM.ManifestSize = 1024 },
		"non-hex":      func(cfg *Config) { cfg.VLLM.ManifestSHA256 = strings.Repeat("z", 64); cfg.VLLM.ManifestSize = 1024 },
		"zero size":    func(cfg *Config) { cfg.VLLM.ManifestSHA256 = strings.Repeat("a", 64) },
		"negative":     func(cfg *Config) { cfg.VLLM.ManifestSHA256 = strings.Repeat("a", 64); cfg.VLLM.ManifestSize = -1 },
	} {
		cfg := Defaults()
		cfg.VLLM.TUFRepositoryURL = ""
		mutate(&cfg)
		if err := validate(&cfg); err == nil {
			t.Fatalf("%s was accepted without a TUF repository", name)
		}
	}
}

func TestValidateRejectsManifestPinAlongsideTUFRepository(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.ManifestSHA256 = strings.Repeat("a", 64)
	cfg.VLLM.ManifestSize = 1024
	if err := validate(&cfg); err == nil {
		t.Fatal("a manifest pin was accepted alongside a TUF repository")
	}
}

func TestValidateStillRequiresCanonicalVLLMTUFRepositoryURL(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.TUFRepositoryURL = "http://example.test/metadata"
	if err := validate(&cfg); err == nil {
		t.Fatal("a plaintext vLLM TUF repository URL was accepted")
	}
}

func TestValidateAcceptsUnverifiedInstallWithNoManifestAtAll(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.TUFRepositoryURL = ""
	cfg.VLLM.ManifestPath = ""
	cfg.VLLM.AllowUnverifiedInstall = true
	cfg.VLLM.UnverifiedVLLMVersion = "0.6.3"
	cfg.VLLM.UnverifiedPythonVersion = "3.12"
	if err := validate(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAcceptsUnverifiedInstallAlongsideTUF(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.AllowUnverifiedInstall = true
	if err := validate(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsUnverifiedOptionsWhenDisabled(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"vllm_version":    func(cfg *Config) { cfg.VLLM.UnverifiedVLLMVersion = "0.6.3" },
		"python_version":  func(cfg *Config) { cfg.VLLM.UnverifiedPythonVersion = "3.12" },
		"index_url":       func(cfg *Config) { cfg.VLLM.UnverifiedIndexURL = "https://pypi.org/simple" },
		"extra_index_url": func(cfg *Config) { cfg.VLLM.UnverifiedExtraIndexURL = "https://pypi.org/simple" },
	} {
		cfg := Defaults()
		mutate(&cfg)
		if err := validate(&cfg); err == nil {
			t.Fatalf("%s was accepted with allow_unverified_install false", name)
		}
	}
}

func TestValidateRejectsUnverifiedInstallDisabledWithNoManifest(t *testing.T) {
	cfg := Defaults()
	cfg.VLLM.TUFRepositoryURL = ""
	cfg.VLLM.ManifestPath = ""
	if err := validate(&cfg); err == nil {
		t.Fatal("expected rejection with no TUF, no pin, and unverified install disabled")
	}
}

func TestLoadExampleConfigParsesSchedulingKeys(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster.SchedulingRefreshInterval != time.Minute {
		t.Fatalf("refresh interval = %v, want 1m", cfg.Cluster.SchedulingRefreshInterval)
	}
	if cfg.Cluster.SchedulingSampleWindow != 24*time.Hour {
		t.Fatalf("sample window = %v, want 24h", cfg.Cluster.SchedulingSampleWindow)
	}
	if cfg.Cluster.SchedulingMinSamples != 20 || cfg.Cluster.SchedulingBackendDepth != 2 {
		t.Fatalf("unexpected scheduling sizing %#v", cfg.Cluster)
	}
	if cfg.Cluster.SchedulingGrantTTL != 30*time.Second {
		t.Fatalf("grant ttl = %v, want 30s", cfg.Cluster.SchedulingGrantTTL)
	}
}

// The fit window has to stay inside the raw retention, because rollups keep totals
// but discard the per-request pairing the fit needs.
func TestSchedulingSampleWindowFitsInsideRawRetention(t *testing.T) {
	cfg := Defaults()
	if cfg.Cluster.SchedulingSampleWindow > cfg.Analytics.RawRetention {
		t.Fatalf("sample window %v exceeds raw retention %v", cfg.Cluster.SchedulingSampleWindow, cfg.Analytics.RawRetention)
	}
}

func TestSchedulingValuesAreValidated(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"zero refresh interval", func(cfg *Config) { cfg.Cluster.SchedulingRefreshInterval = 0 }},
		{"zero sample window", func(cfg *Config) { cfg.Cluster.SchedulingSampleWindow = 0 }},
		{"one sample floor", func(cfg *Config) { cfg.Cluster.SchedulingMinSamples = 1 }},
		{"zero backend depth", func(cfg *Config) { cfg.Cluster.SchedulingBackendDepth = 0 }},
		{"zero grant ttl", func(cfg *Config) { cfg.Cluster.SchedulingGrantTTL = 0 }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := Defaults()
			testCase.mutate(&cfg)
			if err := validate(&cfg); err == nil {
				t.Fatal("invalid scheduling value was accepted")
			}
		})
	}
}

func anyContains(values []string, substr string) bool {
	for _, value := range values {
		if strings.Contains(value, substr) {
			return true
		}
	}
	return false
}
