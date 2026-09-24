package catalog

import "testing"

func TestCapabilitiesContextFallsBackToVLLMMaxModelLength(t *testing.T) {
	metadata := configMetadata{}
	metadata.VLLM.Settings.MaxModelLength = 65536

	capabilities := capabilitiesFromMetadata(metadata, true, false, false, false, false, false)
	if capabilities.Context != 65536 {
		t.Fatalf("context = %d, want the vLLM max_model_length fallback of 65536", capabilities.Context)
	}
}

func TestCapabilitiesContextPrefersContextSizeOverVLLMFallback(t *testing.T) {
	metadata := configMetadata{}
	metadata.ContextSize = 32768
	metadata.VLLM.Settings.MaxModelLength = 65536

	capabilities := capabilitiesFromMetadata(metadata, true, false, false, false, false, false)
	if capabilities.Context != 32768 {
		t.Fatalf("context = %d, want the explicit contextsize, not the vLLM fallback", capabilities.Context)
	}
}

func TestDecodeRuntimeConfigDecodesGPULayersAndToleratesLegacyStreamLayers(t *testing.T) {
	metadata, err := DecodeRuntimeConfig([]byte(`{"gpulayers": -1, "sdstreamlayers": true, "sdstreaming": true}`))
	if err != nil {
		t.Fatalf("stable-diffusion.cpp removed --stream-layers, but configs that still set it must load: %v", err)
	}
	if metadata.GPULayers != -1 {
		t.Fatalf("GPULayers = %d, want -1", metadata.GPULayers)
	}
}

func TestDecodeRuntimeConfigAcceptsLegacyStringReasoningPreserve(t *testing.T) {
	for raw, want := range map[string]bool{`true`: true, `false`: false, `"true"`: true, `"false"`: false, `"on"`: true, `"off"`: false} {
		metadata, err := DecodeRuntimeConfig([]byte(`{"reasoning_preserve": ` + raw + `}`))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if enabled := metadata.ReasoningPreserve.Bool(); enabled == nil || *enabled != want {
			t.Fatalf("%s: reasoning_preserve = %v, want %v", raw, enabled, want)
		}
	}
	if _, err := DecodeRuntimeConfig([]byte(`{"reasoning_preserve": "sometimes"}`)); err == nil {
		t.Fatal("expected a non-boolean reasoning_preserve string to be rejected")
	}
	metadata, err := DecodeRuntimeConfig([]byte(`{"reasoning_preserve": null}`))
	if err != nil || metadata.ReasoningPreserve.Bool() != nil {
		t.Fatalf("null reasoning_preserve must leave the upstream default, got %v, %v", metadata.ReasoningPreserve, err)
	}
}

func TestLlamaLoadModeRejectsValuesOutsideUpstreamList(t *testing.T) {
	for _, mode := range LlamaLoadModes() {
		if resolved, err := (RuntimeConfig{LoadMode: mode}).LlamaLoadMode(); err != nil || resolved != mode {
			t.Fatalf("load_mode %q = %q, %v", mode, resolved, err)
		}
	}
	if _, err := (RuntimeConfig{LoadMode: "direct-io"}).LlamaLoadMode(); err == nil {
		t.Fatal("expected load_mode outside the llama-server --load-mode list to be rejected")
	}
}

func TestDecodeRuntimeConfigRejectsStringGPULayers(t *testing.T) {
	if _, err := DecodeRuntimeConfig([]byte(`{"gpulayers": "auto"}`)); err == nil {
		t.Fatal("expected a decode error for a string gpulayers value")
	}
}
