package proxy

import (
	"net/http"
	"strings"
)

var vllmStaticInferenceRoutes = map[string]map[string]struct{}{
	"/v1/completions":            {http.MethodPost: {}},
	"/v1/chat/completions":       {http.MethodPost: {}},
	"/v1/chat/completions/batch": {http.MethodPost: {}},
	"/v1/responses":              {http.MethodPost: {}},
	"/v1/responses/input_tokens": {http.MethodPost: {}},
	"/v1/embeddings":             {http.MethodPost: {}},
	"/v1/audio/transcriptions":   {http.MethodPost: {}},
	"/v1/audio/translations":     {http.MethodPost: {}},
	"/v1/realtime":               {http.MethodGet: {}},
	"/v1/messages":               {http.MethodPost: {}},
	"/v1/messages/count_tokens":  {http.MethodPost: {}},
	"/v2/embed":                  {http.MethodPost: {}},
	"/rerank":                    {http.MethodPost: {}},
	"/v1/rerank":                 {http.MethodPost: {}},
	"/v2/rerank":                 {http.MethodPost: {}},
	"/classify":                  {http.MethodPost: {}},
	"/score":                     {http.MethodPost: {}},
	"/v1/score":                  {http.MethodPost: {}},
	"/pooling":                   {http.MethodPost: {}},
	"/generative_scoring":        {http.MethodPost: {}},
	"/invocations":               {http.MethodPost: {}},
	"/tokenize":                  {http.MethodPost: {}},
	"/detokenize":                {http.MethodPost: {}},
}

func vllmInferenceAllowed(method string, path string) bool {
	if methods := vllmStaticInferenceRoutes[path]; methods != nil {
		_, ok := methods[method]
		return ok
	}
	responseID, action, ok := vllmResponseOperation(path)
	if !ok || responseID == "" {
		return false
	}
	switch action {
	case "":
		return method == http.MethodGet || method == http.MethodDelete
	case "cancel":
		return method == http.MethodPost
	default:
		return false
	}
}

func vllmResponseOperation(path string) (string, string, bool) {
	if _, static := vllmStaticInferenceRoutes[path]; static {
		return "", "", false
	}
	const prefix = "/v1/responses/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	value := strings.TrimPrefix(path, prefix)
	if value == "" || strings.Contains(value, "\\") {
		return "", "", false
	}
	parts := strings.Split(value, "/")
	if len(parts) > 2 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	if parts[1] != "cancel" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func isVLLMPoolingPath(path string) bool {
	switch path {
	case "/v1/embeddings", "/v2/embed", "/rerank", "/v1/rerank", "/v2/rerank", "/classify", "/score", "/v1/score", "/pooling":
		return true
	default:
		return false
	}
}

func isVLLMTextServingPath(path string) bool {
	if _, found := vllmStaticInferenceRoutes[path]; found {
		return path != "/v1/audio/transcriptions" && path != "/v1/audio/translations" && path != "/v1/realtime"
	}
	_, _, found := vllmResponseOperation(path)
	return found
}

func vllmTaskSupportsPath(task string, path string) bool {
	task = strings.ToLower(strings.TrimSpace(task))
	if path == "/tokenize" || path == "/detokenize" {
		return vllmGenerationTask(task) || vllmPoolingTask(task)
	}
	if _, _, operation := vllmResponseOperation(path); operation {
		return vllmGenerationTask(task)
	}
	switch path {
	case "/v1/completions", "/v1/chat/completions", "/v1/chat/completions/batch", "/v1/responses", "/v1/responses/input_tokens", "/v1/messages", "/v1/messages/count_tokens", "/generative_scoring", "/invocations":
		return vllmGenerationTask(task)
	case "/v1/embeddings", "/v2/embed":
		return task == "embed" || task == "embedding" || task == "embeddings"
	case "/classify":
		return task == "classify" || task == "classification"
	case "/score", "/v1/score", "/rerank", "/v1/rerank", "/v2/rerank":
		return task == "score" || task == "scoring" || task == "reward" || task == "rerank"
	case "/pooling":
		return task == "pooling"
	default:
		return false
	}
}

func vllmGenerationTask(task string) bool {
	switch task {
	case "generate", "generation", "text", "chat", "multimodal", "generative_scoring":
		return true
	default:
		return false
	}
}

func vllmPoolingTask(task string) bool {
	switch task {
	case "embed", "embedding", "embeddings", "classify", "classification", "score", "scoring", "reward", "rerank", "pooling":
		return true
	default:
		return false
	}
}

func vllmReadinessForTask(path string, task string) backendReadiness {
	if vllmPoolingTask(strings.ToLower(strings.TrimSpace(task))) {
		return readinessEmbeddings
	}
	return modelReadiness(path)
}
