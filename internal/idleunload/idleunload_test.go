package idleunload

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimeoutIsOffUnlessConfigured(t *testing.T) {
	for name, options := range map[string]map[string]json.RawMessage{
		"absent": {},
		"null":   {Key: json.RawMessage(`null`)},
		"zero":   {Key: json.RawMessage(`0`)},
	} {
		timeout, err := Timeout(options)
		if err != nil || timeout != 0 {
			t.Fatalf("%s: timeout=%s err=%v, want off", name, timeout, err)
		}
	}
}

func TestTimeoutReadsWholeSeconds(t *testing.T) {
	timeout, err := Timeout(map[string]json.RawMessage{Key: json.RawMessage(` 300 `)})
	if err != nil || timeout != 5*time.Minute {
		t.Fatalf("timeout=%s err=%v, want 5m", timeout, err)
	}
}

func TestParseRejectsValuesThatCannotBeADelay(t *testing.T) {
	for _, raw := range []string{`-1`, `1.5`, `"30"`, `true`, `[30]`, `604801`, `1e30`} {
		if _, err := Parse(json.RawMessage(raw)); err == nil {
			t.Fatalf("Parse(%s) accepted an invalid delay", raw)
		}
	}
}

func TestParseAcceptsTheUpperBound(t *testing.T) {
	timeout, err := Parse(json.RawMessage(`604800`))
	if err != nil || timeout != 7*24*time.Hour {
		t.Fatalf("timeout=%s err=%v, want 168h", timeout, err)
	}
}
