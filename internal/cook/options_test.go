package cook

import (
	"encoding/json"
	"testing"

	"tensors-router/internal/unloadpolicy"
)

func TestFilterOptionsForVoiceMusicKinds(t *testing.T) {
	options := Options{
		"quiet":          rawJSON(t, true),
		"whispermodel":   rawJSON(t, "whisper.gguf"),
		"musicdiffusion": rawJSON(t, "music-diffusion.gguf"),
		"sdmodel":        rawJSON(t, "image.safetensors"),
		"model_param":    rawJSON(t, "text.gguf"),
		"unknown":        rawJSON(t, "custom"),
	}

	filtered := FilterOptionsForKinds(options, []Component{{Kind: KindVoice}, {Kind: KindMusic}})
	if _, ok := filtered["whispermodel"]; !ok {
		t.Fatalf("voice option was filtered out: %#v", filtered)
	}
	if _, ok := filtered["musicdiffusion"]; !ok {
		t.Fatalf("music option was filtered out: %#v", filtered)
	}
	if _, ok := filtered["quiet"]; !ok {
		t.Fatalf("runtime option was filtered out: %#v", filtered)
	}
	if _, ok := filtered["unknown"]; !ok {
		t.Fatalf("unknown option was filtered out: %#v", filtered)
	}
	if _, ok := filtered["sdmodel"]; ok {
		t.Fatalf("image option leaked into voice/music filter: %#v", filtered)
	}
	if _, ok := filtered["model_param"]; ok {
		t.Fatalf("text option leaked into voice/music filter: %#v", filtered)
	}
}

func TestBackendModeOptionAllowsOnlyKnownModes(t *testing.T) {
	mode, ok, err := BackendModeOption(Options{"backend_mode": rawJSON(t, "llama_sdcpp")})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || mode != "llama_sdcpp" {
		t.Fatalf("unexpected backend mode result mode=%q ok=%t", mode, ok)
	}

	_, _, err = BackendModeOption(Options{"backend_mode": rawJSON(t, "native")})
	if err == nil {
		t.Fatalf("expected invalid backend mode error")
	}
}

func TestUnloadPolicyOptionAllowsCurrentTargets(t *testing.T) {
	for _, value := range unloadpolicy.Values() {
		policy, ok, err := UnloadPolicyOption(Options{unloadpolicy.Key: rawJSON(t, value)})
		if err != nil {
			t.Fatalf("expected %q to resolve: %v", value, err)
		}
		if !ok || policy != value {
			t.Fatalf("unexpected unload policy result policy=%q ok=%t", policy, ok)
		}
	}

	_, _, err := UnloadPolicyOption(Options{unloadpolicy.Key: rawJSON(t, "gpu")})
	if err == nil {
		t.Fatal("expected invalid unload policy error")
	}
}

func TestOptionCatalogIncludesUnloadPolicy(t *testing.T) {
	definition, ok := OptionDefinitionForKey(unloadpolicy.Key)
	if !ok {
		t.Fatalf("missing option %q", unloadpolicy.Key)
	}
	if definition.Lane != LaneRuntime {
		t.Fatalf("expected runtime lane, got %q", definition.Lane)
	}
	for _, value := range unloadpolicy.Values() {
		if !containsString(definition.Choices, value) {
			t.Fatalf("missing unload policy choice %q: %#v", value, definition.Choices)
		}
	}
}

func TestOptionCatalogIncludesReleaseCatchUpKeys(t *testing.T) {
	tests := []struct {
		key        string
		backend    string
		nativeFlag string
	}{
		{key: "parallelrequests", backend: "kobold"},
		{key: "parallel", backend: "llama_sdcpp", nativeFlag: "--parallel"},
		{key: "cont_batching", backend: "llama_sdcpp", nativeFlag: "--cont-batching"},
		{key: "cache_ram", backend: "llama_sdcpp", nativeFlag: "--cache-ram"},
		{key: "spec_type", backend: "llama_sdcpp", nativeFlag: "--spec-type"},
		{key: "whispermodel", backend: "llama_sdcpp", nativeFlag: "--model"},
		{key: "talkermodel", backend: "llama_sdcpp", nativeFlag: "--model"},
		{key: "code2wavmodel", backend: "llama_sdcpp", nativeFlag: "--model-vocoder"},
		{key: "sddiffusionmodel", backend: "llama_sdcpp", nativeFlag: "--diffusion-model"},
		{key: "sdbackend", backend: "llama_sdcpp", nativeFlag: "--backend"},
		{key: "sdtensortyperules", backend: "llama_sdcpp", nativeFlag: "--tensor-type-rules"},
		{key: "sdvramlimit", backend: "kobold"},
	}

	for _, testCase := range tests {
		t.Run(testCase.key, func(t *testing.T) {
			definition, ok := OptionDefinitionForKey(testCase.key)
			if !ok {
				t.Fatalf("missing option %q", testCase.key)
			}
			if !containsString(definition.Backends, testCase.backend) {
				t.Fatalf("option %q missing backend %q: %#v", testCase.key, testCase.backend, definition.Backends)
			}
			if definition.NativeFlag != testCase.nativeFlag {
				t.Fatalf("option %q native flag = %q, want %q", testCase.key, definition.NativeFlag, testCase.nativeFlag)
			}
		})
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
