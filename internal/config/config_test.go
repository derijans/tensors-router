package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/offloadsettings"
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
    - "alpha-inference-key-01"
    - "beta-inference-key-002"
  admin_keys:
    - "admin-alpha-key-00001"

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
  token: "cluster-secret-token-01"
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

	for _, field := range overriddenConfigFields(cfg) {
		if !reflect.DeepEqual(field.got, field.want) {
			t.Errorf("%s = %#v, want %#v", field.name, field.got, field.want)
		}
	}
	expectOverrideDeprecationWarnings(t, cfg.Warnings)
}

type loadedConfigField struct {
	name string
	got  any
	want any
}

func overriddenConfigFields(cfg Config) []loadedConfigField {
	fields := []loadedConfigField{
		{"server.bind", cfg.Server.Bind, "127.0.0.1:9999"},
		{"auth.bearer_keys", cfg.Auth.BearerKeys, []string{"alpha-inference-key-01", "beta-inference-key-002"}},
		{"auth.inference_keys", cfg.Auth.InferenceKeys, []string{"alpha-inference-key-01", "beta-inference-key-002"}},
		{"auth.admin_keys", cfg.Auth.AdminKeys, []string{"admin-alpha-key-00001"}},
		{"models.startup_model", cfg.Models.StartupModel, "alpha"},
		{"models.file_roots", cfg.Models.FileRoots, []string{"C:/models", "D:/assets"}},
		{"backend.mode", cfg.Backend.Mode, "llama_sdcpp"},
		{"logging.enabled", cfg.Logging.Enabled, false},
		{"logging.mode", cfg.Logging.Mode, LoggingModeQuiet},
		{"logging.backend_logs_to_disk", cfg.Logging.BackendLogsToDisk, true},
		{"downloader.enabled", cfg.Downloader.Enabled, false},
		{"downloader.binary_location", cfg.Downloader.BinaryLocation, "./tools/tensor-router-downloader"},
		{"analytics.enabled", cfg.Analytics.Enabled, true},
		{"analytics.vram_enabled", cfg.Analytics.VRAMEnabled, false},
		{"analytics.flush_interval", cfg.Analytics.FlushInterval, 2 * time.Minute},
		{"analytics.database_path", cfg.Analytics.DatabasePath, "./store/custom-analytics.sqlite"},
	}
	fields = append(fields, overriddenKoboldFields(cfg.Kobold)...)
	fields = append(fields, overriddenNativeBackendFields(cfg)...)
	fields = append(fields, overriddenUpdateFields(cfg.Updates)...)
	return append(fields, overriddenClusterFields(cfg.Cluster)...)
}

func overriddenKoboldFields(kobold KoboldConfig) []loadedConfigField {
	return []loadedConfigField{
		{"kobold.extra_args", kobold.ExtraArgs, []string{"--flashattention", "--quiet"}},
		{"kobold.multiuser", kobold.Multiuser, 2},
		{"kobold.embeddings_backend_url", kobold.EmbeddingsBackendURL, "http://127.0.0.1:6004"},
		{"kobold.quiet", kobold.Quiet, false},
		{"kobold.skip_launcher", kobold.SkipLauncher, false},
		{"kobold.no_model", kobold.NoModel, false},
		{"kobold.hide_window", kobold.HideWindow, false},
	}
}

func overriddenNativeBackendFields(cfg Config) []loadedConfigField {
	return []loadedConfigField{
		{"llama.backend_url", cfg.Llama.BackendURL, "http://127.0.0.1:6002"},
		{"llama.embeddings_backend_url", cfg.Llama.EmbeddingsBackendURL, "http://127.0.0.1:6005"},
		{"llama.binary_path", cfg.Llama.BinaryPath, "./bin/llama-server"},
		{"llama.data_dir", cfg.Llama.DataDir, "./llama-state"},
		{"llama.hide_window", cfg.Llama.HideWindow, false},
		{"llama.extra_args", cfg.Llama.ExtraArgs, []string{"--parallel", "2"}},
		{"sdcpp.backend_url", cfg.SDCPP.BackendURL, "http://127.0.0.1:7861"},
		{"sdcpp.binary_path", cfg.SDCPP.BinaryPath, "./bin/sd-server"},
		{"sdcpp.data_dir", cfg.SDCPP.DataDir, "./sd-state"},
		{"sdcpp.hide_window", cfg.SDCPP.HideWindow, false},
		{"sdcpp.extra_args", cfg.SDCPP.ExtraArgs, []string{"--verbose"}},
		{"whispercpp.backend_url", cfg.WhisperCPP.BackendURL, "http://127.0.0.1:6003"},
		{"whispercpp.binary_path", cfg.WhisperCPP.BinaryPath, "./bin/whisper-server"},
		{"whispercpp.data_dir", cfg.WhisperCPP.DataDir, "./whisper-state"},
		{"whispercpp.hide_window", cfg.WhisperCPP.HideWindow, false},
		{"whispercpp.extra_args", cfg.WhisperCPP.ExtraArgs, []string{"--language", "auto"}},
	}
}

func overriddenUpdateFields(updates UpdatesConfig) []loadedConfigField {
	return []loadedConfigField{
		{"updates.enabled", updates.Enabled, false},
		{"updates.check_interval", updates.CheckInterval, 24 * time.Hour},
		{"updates.binary_url", updates.BinaryURL, "https://example.test/koboldcpp"},
		{"updates.llama_binary_url", updates.LlamaBinaryURL, "https://example.test/llama-server"},
		{"updates.sdcpp_binary_url", updates.SDCPPBinaryURL, "https://example.test/sd-server"},
		{"updates.whispercpp_binary_url", updates.WhisperCPPBinaryURL, "https://example.test/whisper-server"},
		{"updates.binary_sha256", updates.BinarySHA256, "0000000000000000000000000000000000000000000000000000000000000001"},
		{"updates.llama_binary_sha256", updates.LlamaSHA256, "0000000000000000000000000000000000000000000000000000000000000002"},
		{"updates.sdcpp_binary_sha256", updates.SDCPPSHA256, "0000000000000000000000000000000000000000000000000000000000000003"},
		{"updates.whispercpp_binary_sha256", updates.WhisperCPPSHA256, "0000000000000000000000000000000000000000000000000000000000000004"},
		{"updates.whispercpp_repository_url", updates.WhisperCPPRepositoryURL, "https://github.com/ggml-org/whisper.cpp"},
		{"updates.whispercpp_asset_glob", updates.WhisperCPPAssetGlob, "whisper-bin-x64.zip"},
	}
}

func overriddenClusterFields(cluster ClusterConfig) []loadedConfigField {
	return []loadedConfigField{
		{"cluster.role", cluster.Role, "master"},
		{"cluster.node_id", cluster.NodeID, "master-a"},
		{"cluster.slave_urls", cluster.SlaveURLs, []string{"http://127.0.0.1:8081"}},
		{"cluster.token", cluster.Token, "cluster-secret-token-01"},
		{"cluster.store_dir", cluster.StoreDir, "./store"},
		{"cluster.sync_interval", cluster.SyncInterval, 30 * time.Second},
		{"cluster.health_interval", cluster.HealthInterval, 5 * time.Second},
	}
}

func expectOverrideDeprecationWarnings(t *testing.T, warnings []string) {
	t.Helper()
	if len(warnings) != 5 {
		t.Fatalf("expected 5 compatibility warnings, got %#v", warnings)
	}
	for _, expected := range []string{
		"kobold.embeddings_backend_url is deprecated",
		"llama.embeddings_backend_url is deprecated",
		"analytics.database_path is deprecated",
	} {
		if !anyContains(warnings, expected) {
			t.Errorf("expected warning containing %q, got %#v", expected, warnings)
		}
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
  token: "slave-cluster-token-01"
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

func TestEndpointUniquenessLeavesUnparseableURLsToTheirFieldValidators(t *testing.T) {
	cfg := Defaults()
	cfg.Llama.BackendURL = "not a url"
	cfg.SDCPP.BackendURL = ""
	cfg.WhisperCPP.BackendURL = "http://example.test:5003"
	if err := validateBackendEndpointUniqueness(&cfg); err != nil {
		t.Fatalf("expected unparseable URLs to be skipped, got: %v", err)
	}
	if err := validate(&cfg); err != nil {
		t.Fatalf("expected kobold mode to ignore the split backend URLs, got: %v", err)
	}
	cfg.Backend.Mode = "llama_sdcpp"
	if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "llama.backend_url is invalid") {
		t.Fatalf("expected the llama field validator to report the URL, got: %v", err)
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

func TestLendingValuesTheFileSetsFormTheConfigLayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("cluster:\n  scheduling_context_reserve: 512\n  offload_restore_delay: 1500ms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := (offloadsettings.Values{"scheduling_context_reserve": "512", "offload_restore_delay": "1.5s"}); !reflect.DeepEqual(cfg.Cluster.LendingFileValues, want) {
		t.Fatalf("lending values = %v, want only what the file set, canonicalised: %v", cfg.Cluster.LendingFileValues, want)
	}
}

func TestSchedulingContextReserveRejectsNegative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("cluster:\n  scheduling_context_reserve: -1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "scheduling_context_reserve") {
		t.Fatalf("expected scheduling_context_reserve validation error, got %v", err)
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

func TestLoadExampleConfigStatesTheLendingDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved := offloadsettings.Resolve(cfg.Cluster.LendingFileValues, nil).Settings; resolved != offloadsettings.Defaults() {
		t.Fatalf("example config resolves to %+v, want the built-in defaults %+v", resolved, offloadsettings.Defaults())
	}
}

func TestSchedulingSampleWindowFitsInsideRawRetention(t *testing.T) {
	cfg := Defaults()
	if window := offloadsettings.Defaults().SampleWindow; window > cfg.Analytics.RawRetention {
		t.Fatalf("sample window %v exceeds raw retention %v", window, cfg.Analytics.RawRetention)
	}
}

func TestLendingValuesAreValidatedOnLoad(t *testing.T) {
	for _, line := range []string{
		"scheduling_refresh_interval: 0s",
		"scheduling_sample_window: 0s",
		"scheduling_min_samples: 1",
		"scheduling_backend_depth: 0",
		"scheduling_grant_ttl: 0s",
		"offload_restore_delay: 0s",
		"offload_probe_idle: soon",
	} {
		t.Run(line, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("cluster:\n  "+line+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "cluster.") {
				t.Fatalf("Load accepted %q or reported %v", line, err)
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
