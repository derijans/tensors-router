package proxy

import (
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/recipes"
)

func isImagePath(path string) bool {
	if strings.HasPrefix(path, "/v1/images/") {
		return true
	}
	if strings.HasPrefix(path, "/sdapi/v1/") {
		return true
	}
	if strings.HasPrefix(path, "/sdcpp/v1/") {
		return true
	}
	switch path {
	case "/prompt", "/queue", "/history", "/view", "/object_info", "/system_stats", "/interrupt":
		return true
	default:
		return strings.HasPrefix(path, "/history/") ||
			strings.HasPrefix(path, "/view/") ||
			strings.HasPrefix(path, "/object_info/") ||
			strings.HasPrefix(path, "/upload/image")
	}
}

const sdcppUpscalePath = "/sdcpp/v1/upscale"

func isSDServerModelManagementPath(path string) bool {
	path = strings.TrimRight(path, "/")
	return strings.HasPrefix(path, "/sdcpp/v1/models/") || path == "/sdcpp/v1/model-files" || path == "/sdcpp/v1/recipes"
}

func isCorePath(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/completions"
}

func isEmbeddingsPath(path string) bool {
	return path == "/api/embed" || path == "/api/extra/embeddings" || isVLLMPoolingPath(path)
}

func isVoicePath(path string) bool {
	switch path {
	case pathOpenAISpeech, pathAudioTranscriptions, pathAudioTranslations, "/v1/realtime", "/v1/audio/voices", "/audio/voices", "/api/extra/tts", pathKoboldTranscribe:
		return true
	default:
		return false
	}
}

func isSTTPath(path string) bool {
	return path == pathAudioTranscriptions || path == pathAudioTranslations || path == pathKoboldTranscribe
}

func isMusicPath(path string) bool {
	return path == "/musicui" ||
		strings.HasPrefix(path, "/musicui/") ||
		strings.HasPrefix(path, "/api/extra/music/")
}

func audioRouteKind(path string) string {
	if isMusicPath(path) {
		return recipes.KindMusic
	}
	return recipes.KindVoice
}

func isOpenAIPath(path string) bool {
	return path == "/v1" || strings.HasPrefix(path, "/v1/")
}

func isTextPath(path string) bool {
	if isVLLMTextServingPath(path) {
		return true
	}
	if isOpenAIPath(path) {
		return true
	}
	switch path {
	case "/api/embed":
		return true
	case "/api/v1/generate",
		"/api/extra/generate/stream",
		"/api/extra/embeddings",
		"/api/extra/tokencount",
		pathOllamaGenerate,
		pathOllamaChat,
		"/api/show",
		"/api/tags",
		"/api/ps",
		"/api/version":
		return true
	default:
		return false
	}
}

func textPathRequiresModel(path string) bool {
	if isVLLMTextServingPath(path) {
		_, _, responseOperation := vllmResponseOperation(path)
		return !responseOperation && !selectorlessVLLMPath(path)
	}
	if isCorePath(path) {
		return true
	}
	switch path {
	case "/v1/responses",
		"/v1/responses/input_tokens",
		"/v1/messages",
		"/v1/messages/count_tokens",
		pathRerankV1,
		"/v1/reranking",
		pathOllamaGenerate,
		pathOllamaChat:
		return true
	default:
		return false
	}
}

func isTextInferencePath(path string) bool {
	if isCorePath(path) || isEmbeddingsPath(path) {
		return true
	}
	switch path {
	case pathEmbeddings,
		"/v1/responses",
		"/v1/messages",
		pathRerankV1,
		"/v1/reranking",
		"/api/v1/generate",
		"/api/extra/generate/stream",
		pathOllamaGenerate,
		pathOllamaChat:
		return true
	default:
		return false
	}
}

func isImageDiscoveryPath(path string) bool {
	switch path {
	case "/sdapi/v1/loras",
		"/sdapi/v1/upscalers",
		"/sdapi/v1/schedulers",
		"/sdapi/v1/progress",
		"/sdapi/v1/get_last.json",
		"/sdcpp/v1/capabilities":
		return true
	default:
		return false
	}
}

func modelSupportsLlamaAudioPath(model catalog.Model, path string) bool {
	return voiceSupportsLlamaAudioPath(model.Capabilities.Voice, path)
}

func voiceSupportsLlamaAudioPath(voice *catalog.VoiceCapabilities, path string) bool {
	if voice == nil {
		return false
	}
	switch path {
	case pathOpenAISpeech:
		return catalog.LlamaServesTextToSpeech(voice)
	case pathAudioTranscriptions, pathAudioTranslations, pathKoboldTranscribe:
		return strings.TrimSpace(voice.WhisperModel) != ""
	default:
		return false
	}
}

func joinPath(base string, requestPath string) string {
	if base == "" || base == "/" {
		return requestPath
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(requestPath, "/")
}
