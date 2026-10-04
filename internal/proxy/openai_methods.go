package proxy

import (
	"net/http"

	"tensors-router/internal/openai"
)

func rejectOpenAIMethod(w http.ResponseWriter, r *http.Request) bool {
	allowedMethod, ok := openai.Method(r.URL.Path)
	if !ok || r.Method == allowedMethod {
		return false
	}
	w.Header().Set("Allow", allowedMethod)
	openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	return true
}
