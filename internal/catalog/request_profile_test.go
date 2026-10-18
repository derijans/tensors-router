package catalog

import (
	"encoding/json"
	"testing"
)

var sharingPolicy = RequestProfilePolicy{InjectGenerationDefaults: true, ShareRuntimeByLaunchArguments: true, DefaultBackendMode: "kobold"}

const (
	qwenInstruct = `{"model_param":"/models/qwen.gguf","contextsize":32768,"jinja_kwargs":{"enable_thinking":false},` +
		`"gendefaults":"{\"temperature\": 0.7, \"top_p\": 0.8, \"presence_penalty\": 1.5}"}`
	qwenThink = `{"model_param":"/models/qwen.gguf","contextsize":32768,"jinja_kwargs":{"enable_thinking":true},` +
		`"gendefaults":"{\"temperature\": 1.0, \"top_p\": 0.95, \"presence_penalty\": 0.0}","router_unload_policy":"never"}`
	nemotronInstruct = `{"model_param":"/models/nemotron.gguf","jinja_kwargs":{"enable_thinking":false}}`
	nemotronThink    = `{"model_param":"/models/nemotron.gguf","jinja_kwargs":{"enable_thinking":true}}`
)

func profileFor(t *testing.T, config string, policy RequestProfilePolicy) ChatTemplateProfile {
	t.Helper()
	profile := ChatTemplateProfileForConfig([]byte(config), policy)
	if !profile.Valid() {
		t.Fatalf("profile for %s is invalid", config)
	}
	return profile
}

func requireSharedRuntime(t *testing.T, left string, right string, policy RequestProfilePolicy, want bool) {
	t.Helper()
	leftProfile, rightProfile := profileFor(t, left, policy), profileFor(t, right, policy)
	shared := leftProfile.SharesPhysicalRuntimeWith(rightProfile) && leftProfile.ServableBySiblingRuntime() && rightProfile.ServableBySiblingRuntime()
	if shared != want {
		t.Fatalf("shared runtime = %v, want %v\n%s\n%s", shared, want, left, right)
	}
}

func TestVariantsDifferingInSamplerDefaultsShareARuntimeWhenTheRouterInjectsThem(t *testing.T) {
	requireSharedRuntime(t, qwenInstruct, qwenThink, sharingPolicy, true)
	requireSharedRuntime(t, nemotronInstruct, nemotronThink, sharingPolicy, true)
}

func TestVariantsDifferingInSamplerDefaultsReloadWithTheLegacyFingerprint(t *testing.T) {
	legacy := sharingPolicy
	legacy.ShareRuntimeByLaunchArguments = false

	requireSharedRuntime(t, qwenInstruct, qwenThink, legacy, false)
	requireSharedRuntime(t, nemotronInstruct, nemotronThink, legacy, true)
}

func TestSamplerDefaultsStayInTheFingerprintWhenTheBackendStillAppliesThem(t *testing.T) {
	serverSideDefaults := sharingPolicy
	serverSideDefaults.InjectGenerationDefaults = false

	requireSharedRuntime(t, qwenInstruct, qwenThink, serverSideDefaults, false)
}

func TestVariantsDifferingOnlyInSamplerDefaultsShareWithoutJinjaKwargs(t *testing.T) {
	cold := `{"model_param":"/models/gemma.gguf","gendefaults":{"temperature":0.2}}`
	warm := `{"model_param":"/models/gemma.gguf","gendefaults":{"temperature":1.1}}`

	requireSharedRuntime(t, cold, warm, sharingPolicy, true)
}

func TestConfigsDifferingInContextSizeNeverShare(t *testing.T) {
	smaller := `{"model_param":"/models/qwen.gguf","contextsize":8192,"jinja_kwargs":{"enable_thinking":false}}`
	larger := `{"model_param":"/models/qwen.gguf","contextsize":32768,"jinja_kwargs":{"enable_thinking":false}}`

	requireSharedRuntime(t, smaller, larger, sharingPolicy, false)
}

func TestImageConfigsKeepTheirSamplerDefaultsServerSide(t *testing.T) {
	first := `{"model_param":"/models/text.gguf","sdmodel":"/models/image.safetensors","gendefaults":{"temperature":0.2}}`
	second := `{"model_param":"/models/text.gguf","sdmodel":"/models/image.safetensors","gendefaults":{"temperature":0.9}}`

	requireSharedRuntime(t, first, second, sharingPolicy, false)
	if defaults := profileFor(t, first, sharingPolicy).GenerationDefaults(); !defaults.Empty() {
		t.Fatalf("defaults = %+v, want none injected for a config that also loads an image model", defaults)
	}
}

func TestVLLMConfigsGetNoInjectedSamplerDefaults(t *testing.T) {
	config := `{"backend_mode":"vllm","model_param":"/models/qwen","gendefaults":{"temperature":0.2}}`

	if defaults := profileFor(t, config, sharingPolicy).GenerationDefaults(); !defaults.Empty() {
		t.Fatalf("defaults = %+v, want none for vLLM", defaults)
	}
}

func generationDefaultValues(defaults []GenerationDefault) map[string]string {
	values := map[string]string{}
	for _, item := range defaults {
		values[item.Key] = string(item.Value)
	}
	return values
}

func TestGenerationDefaultsAreReadTheWayKoboldCppReadsThem(t *testing.T) {
	for name, gendefaults := range map[string]string{
		"encoded object":      `"{\"temperature\": 0.7}"`,
		"object":              `{"temperature": 0.7}`,
		"object minus braces": `"\"temperature\": 0.7"`,
	} {
		config := `{"model_param":"/models/qwen.gguf","gendefaults":` + gendefaults + `}`
		defaults := profileFor(t, config, sharingPolicy).GenerationDefaults()
		if got := generationDefaultValues(defaults.TopLevel)["temperature"]; got != "0.7" {
			t.Fatalf("%s: temperature = %q, want 0.7", name, got)
		}
	}
}

func TestMalformedGenerationDefaultsAreIgnoredLikeKoboldCppIgnoresThem(t *testing.T) {
	config := `{"model_param":"/models/qwen.gguf","gendefaults":"temperature:0.35,top_p:0.90"}`

	if defaults := profileFor(t, config, sharingPolicy).GenerationDefaults(); !defaults.Empty() {
		t.Fatalf("defaults = %+v, want none from a value KoboldCpp cannot parse", defaults)
	}
}

func TestGenerationDefaultsOverwriteAcceptsBooleansAndNumbers(t *testing.T) {
	for overwrite, want := range map[string]bool{"true": true, "1": true, "false": false, "0": false} {
		config := `{"model_param":"/models/qwen.gguf","gendefaults":{"temperature":0.7},"gendefaultsoverwrite":` + overwrite + `}`
		if got := profileFor(t, config, sharingPolicy).GenerationDefaults().Overwrite; got != want {
			t.Fatalf("gendefaultsoverwrite %s = %v, want %v", overwrite, got, want)
		}
	}
}

const koboldSpelledDefaults = `{"temperature":0.35,"rep_pen":1.05,"repetition_penalty":1.1,"rep_pen_range":512,"typical":0.9,"top_k":40}`

func TestKoboldGetsItsSamplerNamesUnchanged(t *testing.T) {
	config := `{"backend_mode":"kobold","model_param":"/models/qwen.gguf","gendefaults":` + koboldSpelledDefaults + `}`

	values := generationDefaultValues(profileFor(t, config, sharingPolicy).GenerationDefaults().TopLevel)

	var want map[string]json.RawMessage
	if err := json.Unmarshal([]byte(koboldSpelledDefaults), &want); err != nil {
		t.Fatal(err)
	}
	if len(values) != len(want) || values["rep_pen"] != "1.05" || values["rep_pen_range"] != "512" {
		t.Fatalf("kobold defaults = %v, want the config's own names", values)
	}
}

func TestLlamaGetsLlamaServerSamplerNames(t *testing.T) {
	config := `{"backend_mode":"llama_sdcpp","model_param":"/models/qwen.gguf","gendefaults":` + koboldSpelledDefaults + `}`

	values := generationDefaultValues(profileFor(t, config, sharingPolicy).GenerationDefaults().TopLevel)

	want := map[string]string{"temperature": "0.35", "repeat_penalty": "1.1", "repeat_last_n": "512", "typical_p": "0.9", "top_k": "40"}
	if len(values) != len(want) {
		t.Fatalf("llama defaults = %v, want %v", values, want)
	}
	for key, value := range want {
		if values[key] != value {
			t.Fatalf("llama default %s = %q, want %q (all: %v)", key, values[key], value, values)
		}
	}
}

func TestExplicitLlamaSamplerNameWinsOverTheKoboldSpelling(t *testing.T) {
	config := `{"backend_mode":"llama_sdcpp","model_param":"/models/qwen.gguf","gendefaults":{"typical":0.9,"typical_p":0.5,"rep_pen_range":512,"repeat_last_n":64}}`

	values := generationDefaultValues(profileFor(t, config, sharingPolicy).GenerationDefaults().TopLevel)

	if values["typical_p"] != "0.5" || values["repeat_last_n"] != "64" || values["typical"] != "" || values["rep_pen_range"] != "" {
		t.Fatalf("llama defaults = %v, want the explicit llama-server names", values)
	}
}

func TestOllamaOptionsUseOllamaSamplerNamesOnEveryBackend(t *testing.T) {
	config := `{"backend_mode":"kobold","model_param":"/models/qwen.gguf","gendefaults":{"rep_pen":1.05,"temperature":0.2}}`

	values := generationDefaultValues(profileFor(t, config, sharingPolicy).GenerationDefaults().OllamaOptions)

	if values["repeat_penalty"] != "1.05" || values["temperature"] != "0.2" || values["rep_pen"] != "" {
		t.Fatalf("options defaults = %v, want Ollama names", values)
	}
}

func TestConfigWithoutBackendModeFollowsTheRoutersDefaultFamily(t *testing.T) {
	config := `{"model_param":"/models/qwen.gguf","gendefaults":{"rep_pen":1.05}}`
	llamaDefault := sharingPolicy
	llamaDefault.DefaultBackendMode = "llama_sdcpp"

	values := generationDefaultValues(profileFor(t, config, llamaDefault).GenerationDefaults().TopLevel)

	if values["repeat_penalty"] != "1.05" {
		t.Fatalf("defaults = %v, want llama-server names under a llama_sdcpp router", values)
	}
}
