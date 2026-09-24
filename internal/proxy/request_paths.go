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

func isCorePath(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/completions"
}

func isEmbeddingsPath(path string) bool {
	return path == "/api/embed" || path == "/api/extra/embeddings" || isVLLMPoolingPath(path)
}

func isVoicePath(path string) bool {
	switch path {
	case "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations", "/v1/realtime", "/v1/audio/voices", "/audio/voices", "/api/extra/tts", "/api/extra/transcribe":
		return true
	default:
		return false
	}
}

func isSTTPath(path string) bool {
	return path == "/v1/audio/transcriptions" || path == "/v1/audio/translations" || path == "/api/extra/transcribe"
}

func isTextToSpeechPath(path string) bool {
	return path == "/v1/audio/speech" || path == "/api/extra/tts"
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
		"/api/generate",
		"/api/chat",
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
		"/v1/rerank",
		"/v1/reranking",
		"/api/generate",
		"/api/chat":
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
	case "/v1/embeddings",
		"/v1/responses",
		"/v1/messages",
		"/v1/rerank",
		"/v1/reranking",
		"/api/v1/generate",
		"/api/extra/generate/stream",
		"/api/generate",
		"/api/chat":
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

// modelSupportsLlamaAudioPath deliberately has no text-to-speech case:
// llama.cpp removed --model-vocoder and --model-talker, and llama-server has
// no speech endpoint, so those routes are rejected before they reach here.
func modelSupportsLlamaAudioPath(model catalog.Model, path string) bool {
	if model.Capabilities.Voice == nil {
		return false
	}
	switch path {
	case "/v1/audio/transcriptions", "/v1/audio/translations", "/api/extra/transcribe":
		return strings.TrimSpace(model.Capabilities.Voice.WhisperModel) != ""
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
