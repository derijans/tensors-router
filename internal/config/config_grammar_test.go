package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func loadConfigContent(t *testing.T, content string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestConfigGrammarRejectsMalformedLines(t *testing.T) {
	for name, testCase := range map[string]struct {
		content  string
		expected string
	}{
		"tab indentation":           {"server:\n\tbind: x\n", "line 2: tabs are not supported"},
		"key before any section":    {"bind: x\n", "line 1: expected a section"},
		"line without colon":        {"server:\n  bind\n", "line 2: expected key value"},
		"list item without key":     {"server:\n  - a\n", "line 2: list item without list key"},
		"list item after scalar":    {"server:\n  bind: x\n  - a\n", "line 3: list item without list key"},
		"unknown key":               {"server:\n  nope: true\n", "line 2: unknown key server.nope"},
		"unknown section":           {"nope:\n  key: true\n", "line 2: unknown key nope.key"},
		"empty value on scalar":     {"models:\n  startup_model:\n", "line 2: unknown key models.startup_model"},
		"unterminated single quote": {"server:\n  bind: 'abc\n", "line 2: invalid quoted string"},
		"unterminated double quote": {"server:\n  bind: \"abc\n", "line 2: invalid syntax"},
		"invalid flow list":         {"server:\n  allowed_cidrs: [a, b\n", "line 2: invalid list"},
		"invalid bool":              {"kobold:\n  quiet: maybe\n", `line 2: strconv.ParseBool: parsing "maybe": invalid syntax`},
		"invalid int":               {"kobold:\n  multiuser: two\n", `line 2: strconv.Atoi: parsing "two": invalid syntax`},
		"invalid int64":             {"limits:\n  replay_buffer_mb: 1.5\n", `line 2: strconv.ParseInt: parsing "1.5": invalid syntax`},
		"invalid duration":          {"cluster:\n  sync_interval: soon\n", `line 2: time: invalid duration "soon"`},
		"sdcpp embeddings url":      {"sdcpp:\n  embeddings_backend_url: http://127.0.0.1:0\n", "line 2: unknown key sdcpp.embeddings_backend_url"},
		"whisper embeddings url":    {"whispercpp:\n  embeddings_backend_url: http://127.0.0.1:0\n", "line 2: unknown key whispercpp.embeddings_backend_url"},
		"invalid lending value":     {"cluster:\n  offload_probe_idle: soon\n", `line 2: cluster.offload_probe_idle: time: invalid duration "soon"`},
		"list on scalar key":        {"server:\n  bind: [a]\n", "line 2: unknown key server.bind"},
		"scalar on list key":        {"server:\n  allowed_cidrs: a\n", "line 2: unknown key server.allowed_cidrs"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadConfigContent(t, testCase.content)
			if err == nil || err.Error() != testCase.expected {
				t.Fatalf("expected error %q, got %v", testCase.expected, err)
			}
		})
	}
}

func TestConfigGrammarAcceptsCommentsQuotesAndLists(t *testing.T) {
	cfg, err := loadConfigContent(t, `# leading comment

server:   # section comment
  bind: '127.0.0.1:9999' # quoted then comment
  allowed_cidrs: ["127.0.0.0/8", '::1/128', 10.0.0.0/8]
models:
  config_dir: "./models # not a comment"
  startup_model: alpha#beta
  file_roots:
    - "C:/models"   # item comment
    - D:/assets
kobold:
  extra_args: []
  data_dir: "escaped\tvalue"
llama:
  extra_args: ["a,b", "c"]
sdcpp:
  extra_args:
whispercpp:
  extra_args: [ ]
cluster:
  slave_urls: ["https://node.test/x#frag"]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Bind != "127.0.0.1:9999" {
		t.Fatalf("unexpected bind %q", cfg.Server.Bind)
	}
	if !reflect.DeepEqual(cfg.Server.AllowedCIDRs, []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8"}) {
		t.Fatalf("unexpected allowed CIDRs %#v", cfg.Server.AllowedCIDRs)
	}
	if cfg.Models.ConfigDir != "./models # not a comment" || cfg.Models.StartupModel != "alpha#beta" {
		t.Fatalf("unexpected models %#v", cfg.Models)
	}
	if !reflect.DeepEqual(cfg.Models.FileRoots, []string{"C:/models", "D:/assets"}) {
		t.Fatalf("unexpected file roots %#v", cfg.Models.FileRoots)
	}
	if cfg.Kobold.ExtraArgs == nil || len(cfg.Kobold.ExtraArgs) != 0 || cfg.Kobold.DataDir != "escaped\tvalue" {
		t.Fatalf("unexpected kobold %#v", cfg.Kobold)
	}
	if !reflect.DeepEqual(cfg.Llama.ExtraArgs, []string{"a,b", "c"}) {
		t.Fatalf("unexpected llama args %#v", cfg.Llama.ExtraArgs)
	}
	if cfg.SDCPP.ExtraArgs == nil || len(cfg.SDCPP.ExtraArgs) != 0 || cfg.WhisperCPP.ExtraArgs == nil || len(cfg.WhisperCPP.ExtraArgs) != 0 {
		t.Fatalf("unexpected empty lists %#v %#v", cfg.SDCPP.ExtraArgs, cfg.WhisperCPP.ExtraArgs)
	}
	if !reflect.DeepEqual(cfg.Cluster.SlaveURLs, []string{"https://node.test/x#frag"}) {
		t.Fatalf("unexpected slave URLs %#v", cfg.Cluster.SlaveURLs)
	}
}

func TestConfigGrammarBlockListReplacesDefaults(t *testing.T) {
	cfg, err := loadConfigContent(t, "server:\n  allowed_cidrs:\n    - 127.0.0.0/8\n  bind: 127.0.0.1:9000\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Server.AllowedCIDRs, []string{"127.0.0.0/8"}) || cfg.Server.Bind != "127.0.0.1:9000" {
		t.Fatalf("unexpected server %#v", cfg.Server)
	}
}

func TestConfigGrammarParsesEveryValueType(t *testing.T) {
	cfg, err := loadConfigContent(t, `kobold:
  multiuser: 3
  quiet: f
vllm:
  manifest_size: 42
  manifest_sha256: "0000000000000000000000000000000000000000000000000000000000000000"
  tuf_repository_url: ""
limits:
  drain_timeout: 90s
  separate_runtimes: 2
cluster:
  sync_concurrency: 8
  health_interval: 5s
  offload_probe_idle: 10m
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kobold.Multiuser != 3 || cfg.Kobold.Quiet {
		t.Fatalf("unexpected kobold %#v", cfg.Kobold)
	}
	if cfg.VLLM.ManifestSize != 42 || cfg.Limits.DrainTimeout != 90*time.Second || cfg.Limits.SeparateRuntimes != 2 {
		t.Fatalf("unexpected values %#v %#v", cfg.VLLM, cfg.Limits)
	}
	if cfg.Cluster.SyncConcurrency != 8 || cfg.Cluster.HealthInterval != 5*time.Second || cfg.Cluster.LendingFileValues["offload_probe_idle"] != "10m0s" {
		t.Fatalf("unexpected cluster %#v", cfg.Cluster)
	}
}

func TestConfigGrammarRecordsPresenceFlags(t *testing.T) {
	cfg, err := loadConfigContent(t, `kobold:
  embeddings_backend_url: http://127.0.0.1:0
llama:
  embeddings_backend_url: http://127.0.0.1:0
logging:
  mode: quiet
  enabled: true
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Kobold.embeddingsBackendURLSet || !cfg.Llama.embeddingsBackendURLSet || cfg.SDCPP.embeddingsBackendURLSet {
		t.Fatalf("unexpected embeddings presence %#v %#v", cfg.Kobold, cfg.Llama)
	}
	if !cfg.Logging.modeSet || !cfg.Logging.legacyEnabledSet || cfg.Logging.Mode != LoggingModeQuiet {
		t.Fatalf("unexpected logging %#v", cfg.Logging)
	}
}
