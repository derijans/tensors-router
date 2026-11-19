package cook

import (
	"encoding/json"
	"testing"

	"tensors-router/internal/idleunload"
)

func TestNormalizedOptionsRejectsAnInvalidIdleUnloadDelay(t *testing.T) {
	if _, err := NormalizedOptions(Options{idleunload.Key: json.RawMessage(`-5`)}); err == nil {
		t.Fatal("a negative idle unload delay was written to the config")
	}
}

func TestNormalizedOptionsKeepsAValidIdleUnloadDelay(t *testing.T) {
	normalized, err := NormalizedOptions(Options{idleunload.Key: json.RawMessage(`900`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(normalized[idleunload.Key]) != `900` {
		t.Fatalf("idle unload delay=%s, want 900", normalized[idleunload.Key])
	}
}

func TestOptionCatalogOffersIdleUnloadForEveryBackendFamily(t *testing.T) {
	definition, ok := OptionDefinitionForKey(idleunload.Key)
	if !ok {
		t.Fatalf("missing option %q", idleunload.Key)
	}
	if definition.Lane != LaneRuntime || definition.ValueType != ValueNumber {
		t.Fatalf("lane=%q type=%q, want runtime number", definition.Lane, definition.ValueType)
	}
	if len(definition.Backends) != 3 {
		t.Fatalf("backends=%v, want kobold, llama_sdcpp and vllm", definition.Backends)
	}
}
