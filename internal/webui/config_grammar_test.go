package webui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadWebUIConfigContent(t *testing.T, content string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(path, dir)
}

func TestWebUIConfigGrammarRejectsMalformedLines(t *testing.T) {
	for name, testCase := range map[string]struct {
		content  string
		expected string
	}{
		"tab indentation":           {"server:\n\tbind: x\n", "line 2: tabs are not supported"},
		"key before any section":    {"bind: x\n", "line 1: expected a section"},
		"line without colon":        {"server:\n  bind\n", "line 2: expected key value"},
		"list item without key":     {"server:\n  - a\n", "line 2: list item without list key"},
		"unknown key":               {"server:\n  nope: x\n", "line 2: unknown key server.nope"},
		"unknown section":           {"nope:\n  key: x\n", "line 2: unknown key nope.key"},
		"empty value on scalar":     {"server:\n  bind:\n", "line 2: unknown key server.bind"},
		"unterminated single quote": {"server:\n  bind: 'abc\n", "line 2: invalid quoted string"},
		"unterminated double quote": {"server:\n  bind: \"abc\n", "line 2: invalid syntax"},
		"quoted then comment":       {"server:\n  bind: \"abc\" # note\n", "line 2: invalid syntax"},
		"invalid flow list":         {"router:\n  args: [a, b\n", "line 2: invalid list"},
		"invalid bool":              {"router:\n  start_when_missing: maybe\n", `line 2: strconv.ParseBool: parsing "maybe": invalid syntax`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadWebUIConfigContent(t, testCase.content)
			if err == nil || err.Error() != testCase.expected {
				t.Fatalf("expected error %q, got %v", testCase.expected, err)
			}
		})
	}
}

func TestWebUIConfigGrammarKeepsHashesAndParsesFlowLists(t *testing.T) {
	cfg, err := loadWebUIConfigContent(t, `# leading comment
server:
  cert_hosts: ["a.test", 'b.test', c.test]
router:
  url: https://router.test:8080#frag
`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Server.CertHosts, []string{"a.test", "b.test", "c.test"}) {
		t.Fatalf("unexpected cert hosts %#v", cfg.Server.CertHosts)
	}
	if cfg.Router.URL != "https://router.test:8080#frag" {
		t.Fatalf("unexpected router URL %q", cfg.Router.URL)
	}
}

func TestWebUIConfigGrammarSplitsQuotedCommaIntoBrokenItems(t *testing.T) {
	_, err := loadWebUIConfigContent(t, "router:\n  args: [\"--x,1\"]\n")
	if err == nil || err.Error() != "line 2: invalid syntax" {
		t.Fatalf("expected naive comma split to break the quoted item, got %v", err)
	}
}

func TestWebUIConfigGrammarRecordsLoggingPresence(t *testing.T) {
	cfg, err := loadWebUIConfigContent(t, "logging:\n  mode: quiet\n  enabled: true\n")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Logging.modeSet || !cfg.Logging.legacyEnabledSet || cfg.Logging.Mode != LoggingModeQuiet {
		t.Fatalf("unexpected logging %#v", cfg.Logging)
	}
}
