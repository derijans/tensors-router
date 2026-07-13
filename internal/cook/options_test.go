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

func TestOptionCatalogIncludesCurrentCompatibilityOptions(t *testing.T) {
	tests := []struct {
		key        string
		valueType  string
		nativeFlag string
		legacy     bool
	}{
		{key: "mmproj_auto", valueType: ValueBool, nativeFlag: "--mmproj-auto"},
		{key: "spec_draft_p_min", valueType: ValueNumber, nativeFlag: "--spec-draft-p-min"},
		{key: "sse_ping_interval", valueType: ValueNumber, nativeFlag: "--sse-ping-interval"},
		{key: "sdautofit", valueType: ValueBool, nativeFlag: "--autofit"},
		{key: "sdsplitmode", valueType: ValueString, nativeFlag: "--split-mode"},
		{key: "sdstreaming", valueType: ValueBool, nativeFlag: "--streaming"},
		{key: "sdcircular", valueType: ValueBool, nativeFlag: "--circular"},
		{key: "sdcircularx", valueType: ValueBool, nativeFlag: "--circular-x"},
		{key: "sdcirculary", valueType: ValueBool, nativeFlag: "--circular-y"},
		{key: "sdmaxvram", valueType: ValueString, nativeFlag: "--max-vram"},
		{key: "sdstreamlayers", valueType: ValueNumber, nativeFlag: "--stream-layers", legacy: true},
		{key: "swapadding", valueType: ValueNumber},
	}
	for _, testCase := range tests {
		t.Run(testCase.key, func(t *testing.T) {
			definition, ok := OptionDefinitionForKey(testCase.key)
			if !ok {
				t.Fatalf("missing option %q", testCase.key)
			}
			if definition.ValueType != testCase.valueType || definition.NativeFlag != testCase.nativeFlag || definition.Legacy != testCase.legacy {
				t.Fatalf("unexpected definition %#v", definition)
			}
		})
	}
	if definition, ok := OptionDefinitionForKey("reasoningeffort"); !ok || definition.Default != "default" || !containsString(definition.Choices, "none") {
		t.Fatalf("unexpected reasoning effort definition %#v", definition)
	}
	if definition, ok := OptionDefinitionForKey("defaultgenamt"); !ok || definition.Default != "1536" || !containsString(definition.Choices, "32768") {
		t.Fatalf("unexpected default generation amount definition %#v", definition)
	}
	if definition, ok := OptionDefinitionForKey("sampling_method"); !ok || !containsString(definition.Choices, "dpm++2m_sde_bt") {
		t.Fatalf("missing current sampling method %#v", definition)
	}
	if definition, ok := OptionDefinitionForKey("scheduler"); !ok || !containsString(definition.Choices, "logit_normal") || !containsString(definition.Choices, "beta") {
		t.Fatalf("missing current scheduler choices %#v", definition)
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
