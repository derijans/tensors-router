package openai

import "net/http"

var methods = map[string]string{
	"/v1/chat/completions":      http.MethodPost,
	"/v1/completions":           http.MethodPost,
	"/v1/embeddings":            http.MethodPost,
	"/v1/responses":             http.MethodPost,
	"/v1/messages":              http.MethodPost,
	"/v1/messages/count_tokens": http.MethodPost,
	"/v1/rerank":                http.MethodPost,
	"/v1/reranking":             http.MethodPost,
	"/v1/images/generations":    http.MethodPost,
	"/v1/images/edits":          http.MethodPost,
	"/v1/audio/speech":          http.MethodPost,
	"/v1/audio/transcriptions":  http.MethodPost,
	"/v1/audio/translations":    http.MethodPost,
}

func Method(path string) (string, bool) {
	method, ok := methods[path]
	return method, ok
}
