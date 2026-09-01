package unloadpolicy

import (
	"encoding/json"
	"testing"
)

func TestResolveDefaultsToNoneAndAcceptsCurrentTargets(t *testing.T) {
	if value, err := Resolve(""); err != nil || value != None {
		t.Fatalf("expected default none, got value=%q error=%v", value, err)
	}
	for _, target := range Values() {
		value, err := Resolve(target)
		if err != nil {
			t.Fatalf("expected %q to resolve: %v", target, err)
		}
		if value != target {
			t.Fatalf("expected %q, got %q", target, value)
		}
	}
}

func TestResolveRejectsInvalidPolicy(t *testing.T) {
	if _, err := Resolve("gpu"); err == nil {
		t.Fatal("expected invalid policy to fail")
	}
}

func TestResolveTargetDefaultsToAllAndRejectsNone(t *testing.T) {
	if value, err := ResolveTarget(""); err != nil || value != All {
		t.Fatalf("expected default all, got value=%q error=%v", value, err)
	}
	if _, err := ResolveTarget(None); err == nil {
		t.Fatal("expected none target to fail")
	}
}

func TestResolveRawReportsPresence(t *testing.T) {
	options := map[string]json.RawMessage{Key: json.RawMessage(`"image"`)}
	value, ok, err := ResolveRaw(options)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != Image {
		t.Fatalf("unexpected raw resolution ok=%t value=%q", ok, value)
	}
}

func TestSelectionUnmarshalsLegacyScalarAndArray(t *testing.T) {
	var scalar Selection
	if err := scalar.UnmarshalJSON([]byte(`"all"`)); err != nil {
		t.Fatalf("scalar unmarshal: %v", err)
	}
	resolved, err := ResolveSelection(scalar)
	if err != nil || len(resolved) != 1 || resolved[0] != All {
		t.Fatalf("legacy scalar did not resolve to {all}: %v %v", resolved, err)
	}

	var array Selection
	if err := array.UnmarshalJSON([]byte(`["voice","image"]`)); err != nil {
		t.Fatalf("array unmarshal: %v", err)
	}
	resolved, err = ResolveSelection(array)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved[0] != Image || resolved[1] != Voice {
		t.Fatalf("array did not resolve deduped+sorted: %v", resolved)
	}
}

func TestSelectionMarshalsToArray(t *testing.T) {
	encoded, err := json.Marshal(Selection{Image, Voice})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `["image","voice"]` {
		t.Fatalf("unexpected encoding %s", encoded)
	}
	encoded, err = json.Marshal(Selection(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `[]` {
		t.Fatalf("nil selection should encode as []: %s", encoded)
	}
}

func TestResolveSelectionAbsorbingTriggersRejectCombination(t *testing.T) {
	if _, err := ResolveSelection(Selection{None, Image}); err == nil {
		t.Fatal("expected none combined with a lane to fail")
	}
	if _, err := ResolveSelection(Selection{All, "family:kobold"}); err == nil {
		t.Fatal("expected all combined with a family trigger to fail")
	}
	if resolved, err := ResolveSelection(nil); err != nil || len(resolved) != 1 || resolved[0] != None {
		t.Fatalf("empty selection should resolve to {none}: %v %v", resolved, err)
	}
}

func TestResolveSelectionAcceptsFamilyAndConfigTriggers(t *testing.T) {
	resolved, err := ResolveSelection(Selection{"family:llama_sdcpp", "config:qwen3-1.7b", Text})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 3 {
		t.Fatalf("unexpected resolved set %v", resolved)
	}
	if id, ok := ConfigTarget("config:qwen3-1.7b"); !ok || id != "qwen3-1.7b" {
		t.Fatalf("config target extraction failed: %q %t", id, ok)
	}
	if mode, ok := FamilyTarget("family:llama_sdcpp"); !ok || mode != "llama_sdcpp" {
		t.Fatalf("family target extraction failed: %q %t", mode, ok)
	}
}

func TestResolveSelectionRejectsUnknownTriggers(t *testing.T) {
	if _, err := ResolveSelection(Selection{"family:tensorrt"}); err == nil {
		t.Fatal("expected unknown backend family to be rejected")
	}
	if _, err := ResolveSelection(Selection{"config:"}); err == nil {
		t.Fatal("expected empty config trigger to be rejected")
	}
	if _, err := ResolveSelection(Selection{"gpu"}); err == nil {
		t.Fatal("expected unknown trigger to be rejected")
	}
}
