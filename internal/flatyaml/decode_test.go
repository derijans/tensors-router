package flatyaml

import (
	"reflect"
	"testing"
	"time"
)

type sample struct {
	name     string
	enabled  bool
	count    int
	size     int64
	interval time.Duration
	items    []string
	named    bool
	dynamic  map[string]string
}

func sampleSchema(target *sample) Schema {
	return Schema{
		"main": {
			Fields: Fields{
				"name":     String(&target.name).Marking(&target.named),
				"enabled":  Bool(&target.enabled),
				"count":    Int(&target.count),
				"size":     Int64(&target.size),
				"interval": Duration(&target.interval),
				"items":    StringList(&target.items),
			},
			Dynamic: func(key string) (Field, bool) {
				if key != "extra" {
					return Field{}, false
				}
				return Scalar(func(value string) error {
					target.dynamic[key] = value
					return nil
				}), true
			},
		},
	}
}

var listDialect = Dialect{StripComment: StripTrailingComment, Scalar: UnquoteScalar, SplitListItems: SplitOnUnquotedCommas}

func TestDecodeBindsEveryFieldKind(t *testing.T) {
	var decoded sample
	decoded.dynamic = map[string]string{}
	content := "main:\n  name: 'alpha' # comment\n  enabled: true\n  count: 3\n  size: 9000000000\n  interval: 2s\n  items:\n    - a\n    - \"b\"\n  extra: x\n"
	if err := Decode([]byte(content), listDialect, sampleSchema(&decoded)); err != nil {
		t.Fatal(err)
	}
	expected := sample{name: "alpha", enabled: true, count: 3, size: 9000000000, interval: 2 * time.Second, items: []string{"a", "b"}, named: true, dynamic: map[string]string{"extra": "x"}}
	if !reflect.DeepEqual(decoded, expected) {
		t.Fatalf("decoded %#v, want %#v", decoded, expected)
	}
}

func TestDecodeLeavesPresenceUnmarkedWhenAssignmentFails(t *testing.T) {
	var present bool
	var enabled bool
	schema := Schema{"main": {Fields: Fields{"enabled": Bool(&enabled).Marking(&present)}}}
	if err := Decode([]byte("main:\n  enabled: maybe\n"), listDialect, schema); err == nil || present {
		t.Fatalf("expected a parse error without presence, got err=%v present=%v", err, present)
	}
}

func TestDecodeWithoutListsTreatsEveryValueAsScalar(t *testing.T) {
	var name string
	schema := Schema{"main": {Fields: Fields{"name": String(&name)}}}
	dialect := Dialect{Scalar: UnquoteScalar, NamesUnknownSections: true}
	if err := Decode([]byte("main:\n  name:\n"), dialect, schema); err != nil || name != "" {
		t.Fatalf("expected empty scalar, got %q err=%v", name, err)
	}
	if err := Decode([]byte("main:\n  name: [a]\n"), dialect, schema); err != nil || name != "[a]" {
		t.Fatalf("expected literal bracket scalar, got %q err=%v", name, err)
	}
	if err := Decode([]byte("other:\n  name: x\n"), dialect, schema); err == nil || err.Error() != "line 2: unknown section other" {
		t.Fatalf("expected unknown section error, got %v", err)
	}
}

func TestDecodeReportsValueErrorsBeforeUnknownKeys(t *testing.T) {
	schema := Schema{"main": {}}
	for content, expected := range map[string]string{
		"main:\n  nope: [a\n":  "line 2: invalid list",
		"main:\n  nope: 'a\n":  "line 2: invalid quoted string",
		"main:\n  nope: [a]\n": "line 2: unknown key main.nope",
		"other:\n  nope: x\n":  "line 2: unknown key other.nope",
		"main:\n  - a\n":       "line 2: list item without list key",
		"main:\n  nope\n":      "line 2: expected key value",
		"nope: x\n":            "line 1: expected a section",
		"main:\n\tnope: x\n":   "line 2: tabs are not supported",
		"main:\n  nope: \"a\n": "line 2: invalid syntax",
		"main:\n  nope: x\n  ": "line 2: unknown key main.nope",
	} {
		if err := Decode([]byte(content), listDialect, schema); err == nil || err.Error() != expected {
			t.Fatalf("Decode(%q) error %v, want %q", content, err, expected)
		}
	}
}

func TestSplitOnUnquotedCommasKeepsQuotedCommas(t *testing.T) {
	items := SplitOnUnquotedCommas(`"a,b", 'c,d', e`)
	if !reflect.DeepEqual(items, []string{`"a,b"`, ` 'c,d'`, ` e`}) {
		t.Fatalf("unexpected items %#v", items)
	}
}

func TestStripTrailingCommentKeepsHashesThatAreNotComments(t *testing.T) {
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
		if actual := StripTrailingComment(input); actual != expected {
			t.Fatalf("StripTrailingComment(%q) = %q, want %q", input, actual, expected)
		}
	}
}
