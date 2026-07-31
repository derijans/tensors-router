package proxy

import (
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/recipes"
)

func TestAudioReadinessSelection(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		lane        string
		backendMode string
		want        backendReadiness
	}{
		{name: "speech", path: "/v1/audio/speech", lane: recipes.KindVoice, backendMode: BackendModeKobold, want: readinessSpeech},
		{name: "tts compatibility", path: "/api/extra/tts", lane: recipes.KindVoice, backendMode: BackendModeKobold, want: readinessSpeech},
		{name: "voice discovery", path: "/v1/audio/voices", lane: recipes.KindVoice, backendMode: BackendModeKobold, want: readinessSpeech},
		{name: "transcription", path: "/v1/audio/transcriptions", lane: recipes.KindVoice, backendMode: BackendModeKobold, want: readinessTranscription},
		{name: "translation", path: "/v1/audio/translations", lane: recipes.KindVoice, backendMode: BackendModeKobold, want: readinessTranscription},
		{name: "music", path: "/api/extra/music/generate", lane: recipes.KindMusic, backendMode: BackendModeKobold, want: readinessMusic},
		{name: "llama speech", path: "/v1/audio/speech", lane: recipes.KindVoice, backendMode: BackendModeLlamaSDCPP, want: readinessText},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := audioReadiness(testCase.path, testCase.lane, testCase.backendMode); got != testCase.want {
				t.Fatalf("unexpected readiness %v want %v", got, testCase.want)
			}
		})
	}
}

func TestBackendCapabilityReadinessRequiresEnabledBoolean(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "enabled", body: `{"result":"KoboldCpp","tts":true}`, want: true},
		{name: "disabled", body: `{"result":"KoboldCpp","tts":false}`},
		{name: "missing", body: `{"result":"KoboldCpp"}`},
		{name: "wrong type", body: `{"tts":"true"}`},
		{name: "invalid json", body: `{`},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := backendCapabilityReady(testCase.body, "tts"); got != testCase.want {
				t.Fatalf("unexpected readiness %t want %t", got, testCase.want)
			}
		})
	}
}

func TestBackendReadinessEndpoints(t *testing.T) {
	if readinessText.endpoint() != "/v1/models" {
		t.Fatalf("unexpected text readiness endpoint %q", readinessText.endpoint())
	}
	if readinessImage.endpoint() != "/sdapi/v1/sd-models" {
		t.Fatalf("unexpected image readiness endpoint %q", readinessImage.endpoint())
	}
	for _, readiness := range []backendReadiness{readinessSpeech, readinessTranscription, readinessMusic} {
		if readiness.endpoint() != "/api/extra/version" {
			t.Fatalf("unexpected media readiness endpoint %q", readiness.endpoint())
		}
	}
}

func TestVoiceModelReadinessSelection(t *testing.T) {
	ttsModel := catalog.Model{Capabilities: catalog.Capabilities{Voice: &catalog.VoiceCapabilities{TTSModel: "tts.gguf"}}}
	if got := readinessForVoiceModel(ttsModel, BackendModeKobold); got != readinessSpeech {
		t.Fatalf("unexpected TTS model readiness %v", got)
	}

	transcriptionModel := catalog.Model{Capabilities: catalog.Capabilities{Voice: &catalog.VoiceCapabilities{WhisperModel: "whisper.bin"}}}
	if got := readinessForVoiceModel(transcriptionModel, BackendModeKobold); got != readinessTranscription {
		t.Fatalf("unexpected transcription model readiness %v", got)
	}

	if got := readinessForVoiceModel(ttsModel, BackendModeLlamaSDCPP); got != readinessText {
		t.Fatalf("unexpected llama-sdcpp voice readiness %v", got)
	}
	if got := readinessForVoiceModel(transcriptionModel, BackendModeLlamaSDCPP); got != readinessTranscription {
		t.Fatalf("unexpected split transcription readiness %v", got)
	}
}
