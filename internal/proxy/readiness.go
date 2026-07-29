package proxy

import (
	"encoding/json"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/recipes"
)

type backendReadiness int

const (
	readinessText backendReadiness = iota
	readinessImage
	readinessSpeech
	readinessTranscription
	readinessMusic
)

func (readiness backendReadiness) endpoint() string {
	switch readiness {
	case readinessImage:
		return "/sdapi/v1/sd-models"
	case readinessSpeech, readinessTranscription, readinessMusic:
		return "/api/extra/version"
	default:
		return "/v1/models"
	}
}

func (readiness backendReadiness) capability() string {
	switch readiness {
	case readinessSpeech:
		return "tts"
	case readinessTranscription:
		return "transcribe"
	case readinessMusic:
		return "music"
	default:
		return ""
	}
}

func audioReadiness(path string, lane string, backendMode string) backendReadiness {
	if backendMode != BackendModeKobold {
		return readinessText
	}
	if lane == recipes.KindMusic || isMusicPath(path) {
		return readinessMusic
	}
	switch path {
	case "/v1/audio/transcriptions", "/v1/audio/translations", "/api/extra/transcribe":
		return readinessTranscription
	default:
		return readinessSpeech
	}
}

func readinessForVoiceModel(model catalog.Model, backendMode string) backendReadiness {
	if backendMode != BackendModeKobold {
		return readinessText
	}
	voice := model.Capabilities.Voice
	if voice != nil && strings.TrimSpace(voice.TTSModel) == "" && strings.TrimSpace(voice.WAVTokenizer) == "" && strings.TrimSpace(voice.Directory) == "" && strings.TrimSpace(voice.TalkerModel) == "" && strings.TrimSpace(voice.Code2WAVModel) == "" && strings.TrimSpace(voice.WhisperModel) != "" {
		return readinessTranscription
	}
	return readinessSpeech
}

func backendCapabilityReady(body string, capability string) bool {
	var capabilities map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &capabilities); err != nil {
		return false
	}
	value, ok := capabilities[capability]
	if !ok {
		return false
	}
	var enabled bool
	return json.Unmarshal(value, &enabled) == nil && enabled
}

func routeLaneForReadiness(readiness backendReadiness) string {
	switch readiness {
	case readinessImage:
		return cluster.RouteLaneImage
	case readinessSpeech, readinessTranscription:
		return cluster.RouteLaneVoice
	case readinessMusic:
		return cluster.RouteLaneMusic
	default:
		return cluster.RouteLaneText
	}
}
