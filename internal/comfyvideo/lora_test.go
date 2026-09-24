package comfyvideo

import "testing"

func TestParseWorkflowRejectsLoRANamesThatAreNotPlainNames(t *testing.T) {
	for _, requested := range []string{
		"../outside.safetensors",
		`loras\..\..\outside.safetensors`,
		"/etc/loras/absolute.safetensors",
		`C:\loras\absolute.safetensors`,
		"folder/..",
	} {
		t.Run(requested, func(t *testing.T) {
			graph := Graph{"1": Node{ClassType: "LoraLoaderModelOnly", Inputs: map[string]any{"lora_name": requested, "strength_model": 1.0}}}
			if params, err := ParseWorkflow(graph); err == nil {
				t.Fatalf("LoRA %q must be rejected, got %#v", requested, params.LoRAs)
			}
		})
	}
}

func TestParseWorkflowStripsLoRASubfolders(t *testing.T) {
	graph := Graph{"1": Node{ClassType: "LoraLoader", Inputs: map[string]any{"lora_name": "video/wan/motion.safetensors", "strength_model": 0.5}}}
	params := parseOrFatal(t, graph)
	if len(params.LoRAs) != 1 || params.LoRAs[0].Name != "motion.safetensors" || params.LoRAs[0].Strength != 0.5 {
		t.Fatalf("LoRAs = %#v, want motion.safetensors at 0.5", params.LoRAs)
	}
}
