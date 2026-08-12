package proxy

import (
	"io"
	"net/http"

	"tensors-router/internal/openai"
	"tensors-router/internal/transportbody"
)

func (service *Service) readVLLMRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.ContentLength > service.transportLimits.MaxRequestBytes {
		writeTransportError(w, transportbody.ErrRequestTooLarge)
		return nil, false
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, service.transportLimits.MaxRequestBytes+1))
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
		return nil, false
	}
	if int64(len(body)) > service.transportLimits.MaxRequestBytes {
		writeTransportError(w, transportbody.ErrRequestTooLarge)
		return nil, false
	}
	return body, true
}
