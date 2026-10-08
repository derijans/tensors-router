package downloader

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func loadDownloaderConfigContent(t *testing.T, content string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "downloader.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, _, err := LoadConfig(path)
	return config, err
}

func TestDownloaderConfigGrammarRejectsMalformedLines(t *testing.T) {
	for name, testCase := range map[string]struct {
		content  string
		expected string
	}{
		"tab indentation":        {"storage:\n\troot: x\n", "line 2: tabs are not supported"},
		"key before any section": {"root: x\n", "line 1: expected a section"},
		"line without colon":     {"storage:\n  root\n", "line 2: expected key value"},
		"unknown key":            {"storage:\n  nope: x\n", "line 2: unknown key storage.nope"},
		"unknown section":        {"nope:\n  key: x\n", "line 2: unknown section nope"},
		"unterminated double":    {"huggingface:\n  token: \"abc\n", "line 2: unterminated string"},
		"mismatched quotes":      {"huggingface:\n  token: \"abc'\n", "line 2: unterminated string"},
		"quoted then comment":    {"huggingface:\n  token: \"abc\" # note\n", "line 2: unterminated string"},
		"invalid escape":         {"huggingface:\n  token: \"\\q\"\n", "line 2: invalid syntax"},
		"invalid bool":           {"scanning:\n  write_hash_sidecars: maybe\n", `line 2: strconv.ParseBool: parsing "maybe": invalid syntax`},
		"invalid int64":          {"storage:\n  free_space_reserve_gb: lots\n", `line 2: strconv.ParseInt: parsing "lots": invalid syntax`},
		"invalid duration":       {"downloads:\n  timeout: soon\n", `line 2: time: invalid duration "soon"`},
		"empty int value":        {"hardware:\n  vram_reserve_mb:\n", `line 2: strconv.ParseInt: parsing "": invalid syntax`},
		"list item line":         {"storage:\n  - a\n", "line 2: expected key value"},
		"section comment":        {"storage: # note\n  root: x\n", "line 1: expected a section"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadDownloaderConfigContent(t, testCase.content)
			if err == nil || err.Error() != testCase.expected {
				t.Fatalf("expected error %q, got %v", testCase.expected, err)
			}
		})
	}
}

func TestDownloaderConfigGrammarAcceptsCommentsAndQuotes(t *testing.T) {
	config, err := loadDownloaderConfigContent(t, `# leading comment
huggingface:
  token: 'hf#value # kept'
downloads:
  concurrent_jobs: 3 # trailing comment
  stall_timeout: "2m"
  retry_limit: 0
scanning:
  write_hash_sidecars: false
hardware:
  vram_reserve_mb: 512
logging:
  mode: off
`)
	if err != nil {
		t.Fatal(err)
	}
	if config.HuggingFace.Token != "hf#value # kept" {
		t.Fatalf("unexpected token %q", config.HuggingFace.Token)
	}
	if config.Downloads.ConcurrentJobs != 3 || config.Downloads.StallTimeout != 2*time.Minute || config.Downloads.RetryLimit != 0 {
		t.Fatalf("unexpected downloads %#v", config.Downloads)
	}
	if config.Scanning.WriteHashSidecars || config.Hardware.VRAMReserveMB != 512 || config.Logging.Mode != "off" {
		t.Fatalf("unexpected config %#v %#v %#v", config.Scanning, config.Hardware, config.Logging)
	}
}

func TestDownloaderConfigGrammarTreatsEmptyValueAsEmptyString(t *testing.T) {
	config, err := loadDownloaderConfigContent(t, "huggingface:\n  token:\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.HuggingFace.Token != environmentHubToken() {
		t.Fatalf("unexpected token %q", config.HuggingFace.Token)
	}
}
