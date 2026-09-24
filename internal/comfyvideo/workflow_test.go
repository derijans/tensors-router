package comfyvideo

import "testing"

const wanVideoWorkflow = `{
	"prompt": {
		"1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "wan2.2.safetensors"}},
		"2": {"class_type": "CLIPTextEncode", "inputs": {"text": "a cat riding a skateboard", "clip": ["1", 1]}},
		"3": {"class_type": "CLIPTextEncode", "inputs": {"text": "blurry, low quality", "clip": ["1", 1]}},
		"4": {"class_type": "EmptyHunyuanLatentVideo", "inputs": {"width": 832, "height": 480, "length": 81}},
		"5": {"class_type": "KSampler", "inputs": {
			"seed": 12345, "steps": 20, "cfg": 6.5, "sampler_name": "euler", "scheduler": "simple",
			"positive": ["2", 0], "negative": ["3", 0], "model": ["1", 0], "latent_image": ["4", 0]
		}},
		"6": {"class_type": "WanImageToVideo", "inputs": {"fps": 24, "samples": ["5", 0]}},
		"7": {"class_type": "SaveWEBM", "inputs": {"images": ["6", 0]}}
	}
}`

const stillImageWorkflow = `{
	"1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "sdxl.safetensors"}},
	"2": {"class_type": "CLIPTextEncode", "inputs": {"text": "a mountain landscape", "clip": ["1", 1]}},
	"3": {"class_type": "CLIPTextEncode", "inputs": {"text": "", "clip": ["1", 1]}},
	"4": {"class_type": "EmptyLatentImage", "inputs": {"width": 1024, "height": 1024, "batch_size": 1}},
	"5": {"class_type": "KSampler", "inputs": {
		"seed": 1, "steps": 20, "cfg": 7.0, "sampler_name": "euler", "scheduler": "normal",
		"positive": ["2", 0], "negative": ["3", 0], "model": ["1", 0], "latent_image": ["4", 0]
	}},
	"6": {"class_type": "SaveImage", "inputs": {"images": ["5", 0]}}
}`

func decodeOrFatal(t *testing.T, body string) Graph {
	t.Helper()
	graph, err := DecodeWorkflow([]byte(body))
	if err != nil {
		t.Fatalf("DecodeWorkflow failed: %v", err)
	}
	return graph
}

func parseOrFatal(t *testing.T, graph Graph) Params {
	t.Helper()
	params, err := ParseWorkflow(graph)
	if err != nil {
		t.Fatalf("ParseWorkflow failed: %v", err)
	}
	return params
}

func TestDecodeWorkflowAcceptsEnvelopeAndBareGraph(t *testing.T) {
	envelope := decodeOrFatal(t, wanVideoWorkflow)
	if len(envelope) != 7 {
		t.Fatalf("expected 7 nodes from envelope form, got %d", len(envelope))
	}
	bare := decodeOrFatal(t, stillImageWorkflow)
	if len(bare) != 6 {
		t.Fatalf("expected 6 nodes from bare form, got %d", len(bare))
	}
}

func TestDecodeWorkflowRejectsInvalidAndEmptyInput(t *testing.T) {
	if _, err := DecodeWorkflow([]byte("not json")); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
	if _, err := DecodeWorkflow([]byte("{}")); err == nil {
		t.Fatal("expected an error for an empty graph")
	}
}

func TestIsVideoWorkflowDetectsKnownVideoNodeNames(t *testing.T) {
	if !IsVideoWorkflow(decodeOrFatal(t, wanVideoWorkflow)) {
		t.Fatal("expected the Wan workflow to be detected as video")
	}
}

func TestIsVideoWorkflowDetectsFrameCountAboveOne(t *testing.T) {
	graph := Graph{"1": Node{ClassType: "SomeUnknownVideoLatent", Inputs: map[string]any{"length": 81.0}}}
	if !IsVideoWorkflow(graph) {
		t.Fatal("expected a frame count above one to mark the workflow as video")
	}
}

func TestIsVideoWorkflowRejectsAPlainImageWorkflow(t *testing.T) {
	if IsVideoWorkflow(decodeOrFatal(t, stillImageWorkflow)) {
		t.Fatal("a single-frame still-image workflow must not be classified as video")
	}
}

func TestParseWorkflowExtractsPromptsAndSamplerSettings(t *testing.T) {
	params := parseOrFatal(t, decodeOrFatal(t, wanVideoWorkflow))
	if params.Prompt != "a cat riding a skateboard" {
		t.Fatalf("unexpected prompt %q", params.Prompt)
	}
	if params.NegativePrompt != "blurry, low quality" {
		t.Fatalf("unexpected negative prompt %q", params.NegativePrompt)
	}
	if params.Width != 832 || params.Height != 480 {
		t.Fatalf("unexpected dimensions %dx%d", params.Width, params.Height)
	}
	if params.Frames != 81 {
		t.Fatalf("unexpected frame count %d", params.Frames)
	}
	if params.FPS != 24 {
		t.Fatalf("unexpected fps %d", params.FPS)
	}
	if params.Seed != 12345 || params.Steps != 20 || params.CFG != 6.5 {
		t.Fatalf("unexpected sampler settings %#v", params)
	}
	if params.SamplerName != "euler" || params.Scheduler != "simple" {
		t.Fatalf("unexpected sampler name/scheduler %#v", params)
	}
}

func TestParseWorkflowFallsBackToDefaultsWithoutASamplerNode(t *testing.T) {
	graph := Graph{"1": Node{ClassType: "Note", Inputs: map[string]any{}}}
	params := parseOrFatal(t, graph)
	if params.Width != 512 || params.Height != 512 || params.Frames != 1 || params.FPS != 16 {
		t.Fatalf("expected default params, got %#v", params)
	}
	if params.Prompt != "" || params.NegativePrompt != "" {
		t.Fatalf("expected empty prompts without a sampler node, got %#v", params)
	}
}

func TestParseWorkflowAcceptsAnInlineLiteralPrompt(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "KSampler", Inputs: map[string]any{
			"positive": "an inline literal prompt",
			"negative": "",
		}},
	}
	params := parseOrFatal(t, graph)
	if params.Prompt != "an inline literal prompt" {
		t.Fatalf("expected the inline literal prompt to pass through, got %q", params.Prompt)
	}
}

func TestIsVideoWorkflowDetectsLTXVAndWanSoundToVideoNodes(t *testing.T) {
	for _, classType := range []string{"EmptyLTXVLatentVideo", "LTXVImgToVideo", "LTXVConditioning", "WanSoundImageToVideo"} {
		t.Run(classType, func(t *testing.T) {
			graph := Graph{"1": Node{ClassType: classType, Inputs: map[string]any{}}}
			if !IsVideoWorkflow(graph) {
				t.Fatalf("%s must mark the workflow as video without a frame-count input", classType)
			}
		})
	}
}
