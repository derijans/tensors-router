package comfyvideo

import (
	"slices"
	"testing"
)

func link(nodeID string) []any {
	return []any{nodeID, float64(0)}
}

func TestParseWorkflowTakesTheStartFrameFromAStartImageInput(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "uploaded-frame.png"}},
		"2": Node{ClassType: "WanImageToVideo", Inputs: map[string]any{"length": float64(9), "start_image": link("1")}},
	}
	params := parseOrFatal(t, graph)
	if params.StartFrame != "uploaded-frame.png" {
		t.Fatalf("start frame = %q, want the uploaded name", params.StartFrame)
	}
	if len(params.ReferenceImages) != 0 {
		t.Fatalf("a wired start frame must not also be a reference image, got %v", params.ReferenceImages)
	}
}

func TestParseWorkflowIgnoresALoadImageFedByAGeneratedImage(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "VAEDecode", Inputs: map[string]any{"samples": link("9")}},
		"2": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": link("1")}},
		"3": Node{ClassType: "SaveWEBM", Inputs: map[string]any{"images": link("2")}},
	}
	params := parseOrFatal(t, graph)
	if params.StartFrame != "" || len(params.ReferenceImages) != 0 {
		t.Fatalf("a linked LoadImage input is not an upload, got start=%q refs=%v", params.StartFrame, params.ReferenceImages)
	}
}

func TestParseWorkflowReportsNoMediaForTextToVideo(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "EmptyHunyuanLatentVideo", Inputs: map[string]any{"width": float64(64), "height": float64(64), "length": float64(9)}},
		"2": Node{ClassType: "SaveWEBM", Inputs: map[string]any{"images": link("1")}},
	}
	params := parseOrFatal(t, graph)
	if params.StartFrame != "" || params.EndFrame != "" || len(params.ReferenceImages) != 0 || len(params.ReferenceAudios) != 0 || len(params.LoRAs) != 0 {
		t.Fatalf("text-to-video must carry no media, got %#v", params)
	}
}

func TestParseWorkflowCollectsFramesReferencesAudiosAndLoRAs(t *testing.T) {
	graph := Graph{
		"2":  Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "ref-a.png"}},
		"10": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "ref-b.png"}},
		"3":  Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "start.png"}},
		"4":  Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "end.png"}},
		"5":  Node{ClassType: "ImageScale", Inputs: map[string]any{"image": link("3"), "width": float64(832)}},
		"6":  Node{ClassType: "WanFirstLastFrameToVideo", Inputs: map[string]any{"start_image": link("5"), "end_image": link("4"), "length": float64(33)}},
		"7":  Node{ClassType: "LoadAudio", Inputs: map[string]any{"audio": "voice.wav"}},
		"8":  Node{ClassType: "VHS_LoadAudio", Inputs: map[string]any{"audio_file": "music.mp3"}},
		"9":  Node{ClassType: "LoraLoader", Inputs: map[string]any{"lora_name": `wan\motion.safetensors`, "strength_model": 0.8, "strength_clip": 1.0}},
		"11": Node{ClassType: "LoraLoaderModelOnly", Inputs: map[string]any{"lora_name": "detail.safetensors"}},
	}
	params := parseOrFatal(t, graph)

	if params.StartFrame != "start.png" || params.EndFrame != "end.png" {
		t.Fatalf("frames = %q..%q, want start.png..end.png followed through the resize node", params.StartFrame, params.EndFrame)
	}
	if want := []string{"ref-a.png", "ref-b.png"}; !slices.Equal(params.ReferenceImages, want) {
		t.Fatalf("reference images = %v, want %v in node order", params.ReferenceImages, want)
	}
	if want := []string{"voice.wav", "music.mp3"}; !slices.Equal(params.ReferenceAudios, want) {
		t.Fatalf("reference audios = %v, want %v", params.ReferenceAudios, want)
	}
	if want := []LoRA{{Name: "motion.safetensors", Strength: 0.8}, {Name: "detail.safetensors", Strength: 1}}; !slices.Equal(params.LoRAs, want) {
		t.Fatalf("LoRAs = %#v, want %#v", params.LoRAs, want)
	}
}

func TestParseWorkflowUsesTheFirstReferenceAsStartFrameWhenNoneIsWired(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "subject.png"}},
		"2": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "style.png"}},
		"3": Node{ClassType: "MiniMaxH3Reference", Inputs: map[string]any{"reference_1": link("1"), "reference_2": link("2")}},
	}
	params := parseOrFatal(t, graph)
	if params.StartFrame != "subject.png" {
		t.Fatalf("start frame = %q, want the first reference", params.StartFrame)
	}
	if want := []string{"subject.png", "style.png"}; !slices.Equal(params.ReferenceImages, want) {
		t.Fatalf("reference images = %v, want %v", params.ReferenceImages, want)
	}
}

func TestParseWorkflowTreatsTheImageInputOfAnImageToVideoNodeAsTheStartFrame(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "first.png"}},
		"2": Node{ClassType: "LTXVImgToVideo", Inputs: map[string]any{"image": link("1"), "length": float64(97)}},
	}
	params := parseOrFatal(t, graph)
	if params.StartFrame != "first.png" || len(params.ReferenceImages) != 0 {
		t.Fatalf("got start=%q refs=%v, want first.png as the start frame only", params.StartFrame, params.ReferenceImages)
	}
}

func TestParseWorkflowKeepsTheSoundToVideoReferenceImageAsAReference(t *testing.T) {
	graph := Graph{
		"1": Node{ClassType: "LoadImage", Inputs: map[string]any{"image": "speaker.png"}},
		"2": Node{ClassType: "LoadAudio", Inputs: map[string]any{"audio": "speech.flac"}},
		"3": Node{ClassType: "WanSoundImageToVideo", Inputs: map[string]any{"ref_image": link("1"), "length": float64(77)}},
	}
	params := parseOrFatal(t, graph)
	if !slices.Equal(params.ReferenceImages, []string{"speaker.png"}) || !slices.Equal(params.ReferenceAudios, []string{"speech.flac"}) {
		t.Fatalf("got refs=%v audios=%v", params.ReferenceImages, params.ReferenceAudios)
	}
}
