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
