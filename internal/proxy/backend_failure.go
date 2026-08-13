package proxy

import (
	"net/http"
	"strings"

	"tensors-router/internal/openai"
)

func backendFailureResponse(err error) (int, string, string) {
	if err != nil && strings.Contains(err.Error(), "backend_not_initialized") {
		return http.StatusServiceUnavailable, "backend_not_initialized", "vLLM backend is not initialized"
	}
	return http.StatusBadGateway, "backend_error", err.Error()
}

func writeBackendFailure(w http.ResponseWriter, err error) int {
	status, code, message := backendFailureResponse(err)
	openai.WriteError(w, status, code, message)
	return status
}
