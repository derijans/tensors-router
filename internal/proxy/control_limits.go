package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/openai"
	"tensors-router/internal/transportbody"
)

func (service *Service) limitControlRequestBody(w http.ResponseWriter, r *http.Request) bool {
	if !controlRequestHasBody(r) {
		return false
	}
	limit := service.maxControlBodyBytes
	if r.ContentLength > limit {
		writeTransportError(w, transportbody.ErrRequestTooLarge)
		return true
	}
	content, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	_ = r.Body.Close()
	if err != nil {
		writeTransportError(w, err)
		return true
	}
	if int64(len(content)) > limit {
		writeTransportError(w, transportbody.ErrRequestTooLarge)
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(content))
	r.ContentLength = int64(len(content))
	return false
}

func controlRequestHasBody(r *http.Request) bool {
	if r == nil || r.Body == nil {
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/router/v1/") && !inferenceControlPath(r.URL.Path) {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/router/v1/node/inference/") {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func (service *Service) rejectModelLoadWhileDraining(w http.ResponseWriter) bool {
	if !service.Draining() {
		return false
	}
	openai.WriteError(w, http.StatusServiceUnavailable, "router_draining", "router is draining and cannot accept model loads")
	return true
}
