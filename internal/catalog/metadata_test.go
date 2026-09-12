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

func TestDecodeRuntimeConfigDecodesGPULayersAndStreamLayers(t *testing.T) {
	metadata, err := DecodeRuntimeConfig([]byte(`{"gpulayers": -1, "sdstreamlayers": true}`))
	if err != nil {
		t.Fatalf("DecodeRuntimeConfig failed: %v", err)
	}
	if metadata.GPULayers != -1 {
		t.Fatalf("GPULayers = %d, want -1", metadata.GPULayers)
	}
	if !metadata.SDStreamLayers {
		t.Fatalf("SDStreamLayers = false, want true")
	}
}

func TestDecodeRuntimeConfigRejectsStringGPULayers(t *testing.T) {
	if _, err := DecodeRuntimeConfig([]byte(`{"gpulayers": "auto"}`)); err == nil {
		t.Fatal("expected a decode error for a string gpulayers value")
	}
}
