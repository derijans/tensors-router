package cook

import (
	"encoding/json"
	"testing"

	"tensors-router/internal/catalog"
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
	for _, expected := range []string{"llama_sdcpp", "vllm"} {
		mode, ok, err := BackendModeOption(Options{"backend_mode": rawJSON(t, expected)})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || mode != expected {
			t.Fatalf("unexpected backend mode result mode=%q ok=%t", mode, ok)
		}
	}

	_, _, err := BackendModeOption(Options{"backend_mode": rawJSON(t, "native")})
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
		if !ok || len(policy) != 1 || policy[0] != value {
			t.Fatalf("unexpected unload policy result policy=%v ok=%t", policy, ok)
		}
	}

	multi, ok, err := UnloadPolicyOption(Options{unloadpolicy.Key: json.RawMessage(`["image","family:kobold"]`)})
	if err != nil || !ok || len(multi) != 2 {
		t.Fatalf("array unload policy did not resolve: policy=%v ok=%t err=%v", multi, ok, err)
	}

	if _, _, err := UnloadPolicyOption(Options{unloadpolicy.Key: rawJSON(t, "gpu")}); err == nil {
		t.Fatal("expected invalid unload policy error")
	}
	if _, _, err := UnloadPolicyOption(Options{unloadpolicy.Key: json.RawMessage(`["none","image"]`)}); err == nil {
		t.Fatal("expected none combined with a lane to fail")
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

func TestOptionCatalogIncludesVLLMRuntimeSection(t *testing.T) {
	definition, ok := OptionDefinitionForKey("vllm")
	if !ok || definition.ValueType != ValueJSON || len(definition.Backends) != 1 || definition.Backends[0] != "vllm" {
		t.Fatalf("unexpected vLLM option definition %#v", definition)
	}
}

func TestNormalizedOptionsCanonicalizesJinjaKwargsForConfigPersistence(t *testing.T) {
	options, err := NormalizedOptions(Options{
		catalog.JinjaKwargsKey:                 rawJSON(t, map[string]any{"enable_thinking": true, "mode": "chat"}),
		catalog.RouterJinjaKwargsPrecedenceKey: rawJSON(t, "client"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := json.Unmarshal(options[catalog.JinjaKwargsKey], &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded != `{"enable_thinking":true,"mode":"chat"}` {
		t.Fatalf("unexpected encoded Jinja kwargs %q", encoded)
	}
	var precedence string
	if err := json.Unmarshal(options[catalog.RouterJinjaKwargsPrecedenceKey], &precedence); err != nil {
		t.Fatal(err)
	}
	if precedence != catalog.JinjaKwargsPrecedenceClient {
		t.Fatalf("unexpected precedence %q", precedence)
	}

	options, err = NormalizedOptions(Options{
		catalog.JinjaKwargsKey: rawJSON(t, `{"enable_thinking":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(options[catalog.JinjaKwargsKey], &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded != `{"enable_thinking":false}` {
		t.Fatalf("unexpected encoded string kwargs %q", encoded)
	}
}

func TestNormalizedOptionsRejectsInvalidJinjaKwargsAndPrecedence(t *testing.T) {
	if _, err := NormalizedOptions(Options{catalog.JinjaKwargsKey: rawJSON(t, []any{"not", "an", "object"})}); err == nil {
		t.Fatal("expected non-object Jinja kwargs to be rejected")
	}
	if _, err := NormalizedOptions(Options{catalog.RouterJinjaKwargsPrecedenceKey: rawJSON(t, "backend")}); err == nil {
		t.Fatal("expected invalid Jinja precedence to be rejected")
	}
}

func TestOptionCatalogIncludesJinjaKwargsPrecedence(t *testing.T) {
	definition, ok := OptionDefinitionForKey(catalog.RouterJinjaKwargsPrecedenceKey)
	if !ok {
		t.Fatalf("missing option %q", catalog.RouterJinjaKwargsPrecedenceKey)
	}
	if definition.Default != catalog.JinjaKwargsPrecedenceConfig || !containsString(definition.Choices, catalog.JinjaKwargsPrecedenceClient) {
		t.Fatalf("unexpected Jinja precedence definition %#v", definition)
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
		{key: "talkermodel", backend: "kobold"},
		{key: "code2wavmodel", backend: "kobold"},
		{key: "sddiffusionmodel", backend: "llama_sdcpp", nativeFlag: "--diffusion-model"},
		{key: "sdbackend", backend: "llama_sdcpp", nativeFlag: "--backend"},
		{key: "sdtensortyperules", backend: "llama_sdcpp", nativeFlag: "--tensor-type-rules"},
		{key: "sdvramlimit", backend: "kobold"},
		{key: "sdaudiovae", backend: "llama_sdcpp", nativeFlag: "--audio-vae"},
		{key: "sdphotomaker", backend: "llama_sdcpp", nativeFlag: "--photo-maker"},
		{key: "mmproj_device", backend: "llama_sdcpp", nativeFlag: "--mmproj-device"},
		{key: "reasoning_effort", backend: "llama_sdcpp", nativeFlag: "--reasoning-effort"},
		{key: "tools_runtime", backend: "llama_sdcpp", nativeFlag: "--tools-runtime"},
		{key: "sampling_method", backend: "llama_sdcpp", nativeFlag: "--sampling-method"},
		{key: "high_noise_sampling_method", backend: "llama_sdcpp", nativeFlag: "--high-noise-sampling-method"},
		{key: "scheduler", backend: "llama_sdcpp", nativeFlag: "--scheduler"},
		{key: "type", backend: "llama_sdcpp", nativeFlag: "--type"},
		{key: "rng", backend: "llama_sdcpp", nativeFlag: "--rng"},
		{key: "sampler_rng", backend: "llama_sdcpp", nativeFlag: "--sampler-rng"},
		{key: "prediction", backend: "llama_sdcpp", nativeFlag: "--prediction"},
		{key: "lora_apply_mode", backend: "llama_sdcpp", nativeFlag: "--lora-apply-mode"},
		{key: "cache_mode", backend: "llama_sdcpp", nativeFlag: "--cache-mode"},
		{key: "cache_option", backend: "llama_sdcpp", nativeFlag: "--cache-option"},
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
		{key: "sdautofit", valueType: ValueBool, nativeFlag: "--auto-fit"},
		{key: "sdsplitmode", valueType: ValueString, nativeFlag: "--split-mode"},
		{key: "sdstreaming", valueType: ValueBool, legacy: true},
		{key: "sdcircular", valueType: ValueBool, nativeFlag: "--circular"},
		{key: "sdcircularx", valueType: ValueBool, nativeFlag: "--circularx"},
		{key: "sdcirculary", valueType: ValueBool, nativeFlag: "--circulary"},
		{key: "sdmaxvram", valueType: ValueString, nativeFlag: "--max-vram"},
		{key: "sdstreamlayers", valueType: ValueBool, legacy: true},
		{key: "sdllmvision", valueType: ValueString, nativeFlag: "--llm_vision"},
		{key: "sdclipvision", valueType: ValueString, nativeFlag: "--clip_vision"},
		{key: "sdembeddingsconnectors", valueType: ValueJSON, nativeFlag: "--embeddings-connectors"},
		{key: "sdhiresupscalersdir", valueType: ValueString, nativeFlag: "--hires-upscalers-dir"},
		{key: "sdtokenizer", valueType: ValueString, nativeFlag: "--tokenizer"},
		{key: "sdaudioencoder", valueType: ValueString, nativeFlag: "--audio-encoder"},
		{key: "sdconditioningcachesize", valueType: ValueNumber, nativeFlag: "--conditioning-cache-size"},
		{key: "sdimagepreprocess", valueType: ValueJSON, nativeFlag: "--image-preprocess"},
		{key: "load_mode", valueType: ValueString, nativeFlag: "--load-mode"},
		{key: "usemmap", valueType: ValueBool, nativeFlag: "--load-mode"},
		{key: "usemlock", valueType: ValueBool, nativeFlag: "--load-mode"},
		{key: "reasoning_preserve", valueType: ValueBool, nativeFlag: "--reasoning-preserve"},
		{key: "draft_dflash", valueType: ValueBool, nativeFlag: "--spec-type"},
		{key: "draft_dspark", valueType: ValueBool, nativeFlag: "--spec-type"},
		{key: "draftgpulayers", valueType: ValueNumber, nativeFlag: "--spec-draft-ngl"},
		{key: "draftamount", valueType: ValueNumber, nativeFlag: "--spec-draft-n-max"},
		{key: "n_cpu_ffn", valueType: ValueNumber, nativeFlag: "--n-cpu-ffn"},
		{key: "lazy_mode", valueType: ValueString, nativeFlag: "--lazy-mode"},
		{key: "kv_unified_per_slot", valueType: ValueNumber, nativeFlag: "--kv-unified-per-slot"},
		{key: "video_fps", valueType: ValueNumber, nativeFlag: "--video-fps"},
		{key: "video_timestamp_interval", valueType: ValueNumber, nativeFlag: "--video-timestamp-interval"},
		{key: "whispercpp_no_context", valueType: ValueBool, legacy: true},
		{key: "usedirectio", valueType: ValueBool},
		{key: "ffncpu", valueType: ValueNumber},
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
	if definition, ok := OptionDefinitionForKey("reasoningeffort"); !ok || definition.Default != "default" || !containsString(definition.Choices, "none") || !containsString(definition.Choices, "xhigh") {
		t.Fatalf("unexpected reasoning effort definition %#v", definition)
	}
	if definition, ok := OptionDefinitionForKey("defaultgenamt"); !ok || definition.Default != "1536" || !containsString(definition.Choices, "32768") {
		t.Fatalf("unexpected default generation amount definition %#v", definition)
	}
	if definition, ok := OptionDefinitionForKey("sampling_method"); !ok || !containsString(definition.Choices, "dpm++2m_sde_bt") {
		t.Fatalf("missing current sampling method %#v", definition)
	}
	if definition, ok := OptionDefinitionForKey("scheduler"); !ok || !containsString(definition.Choices, "logit_normal") || !containsString(definition.Choices, "beta") || !containsString(definition.Choices, "llada_image") {
		t.Fatalf("missing current scheduler choices %#v", definition)
	}
	for key, expected := range map[string][]string{
		"load_mode":        {"auto", "none", "mmap", "mlock", "mmap+mlock", "dio"},
		"lazy_mode":        {"on", "auto", "off"},
		"reasoning_effort": {"default", "xhigh", "max"},
		"type":             {"q2_0", "f8_e4m3", "f8_e5m2"},
		"prediction":       {"sefi_flow", "sensenova_u1_flow"},
		"spec_type":        {"draft-dflash", "draft-dspark"},
		"sdloglevel":       {"debug", "verbose", "info", "warn", "error"},
		"device":           {"none", "CPU", "CUDA0", "Vulkan0", "CUDA0,CUDA1"},
	} {
		definition, ok := OptionDefinitionForKey(key)
		if !ok {
			t.Fatalf("missing option %q", key)
		}
		for _, choice := range expected {
			if !containsString(definition.Choices, choice) {
				t.Fatalf("option %q missing upstream choice %q: %#v", key, choice, definition.Choices)
			}
		}
	}
	if definition, _ := OptionDefinitionForKey("device"); containsString(definition.Choices, "cuda") || containsString(definition.Choices, "vulkan") {
		t.Fatalf("llama.cpp --device takes backend device names such as CUDA0, not backend families: %#v", definition.Choices)
	}
	if definition, _ := OptionDefinitionForKey("prediction"); containsString(definition.Choices, "flux2_flow") {
		t.Fatalf("sd.cpp master-908 prediction_to_str has no flux2_flow: %#v", definition.Choices)
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
