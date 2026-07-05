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
