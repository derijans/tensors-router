package cook

import (
	"encoding/json"
	"testing"
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

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
