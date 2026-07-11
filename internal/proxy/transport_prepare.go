package proxy

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"tensors-router/internal/openai"
	"tensors-router/internal/transportbody"
)

func (service *Service) reserveTransportWorkingSet(r *http.Request) (*transportbody.Reservation, bool) {
	if !transportInferenceRequest(r) {
		return nil, true
	}
	if r.ContentLength > service.transportLimits.MaxRequestBytes {
		return nil, true
	}
	selectorThreshold := service.transportLimits.ReplayBufferBytes
	if service.transportLimits.SelectorScanBytes < selectorThreshold {
		selectorThreshold = service.transportLimits.SelectorScanBytes
	}
	if r.ContentLength > selectorThreshold && transportExternalSelector(r) == "" {
		return nil, true
	}
	retainedBytes := service.transportLimits.ReplayBufferBytes
	if r.ContentLength >= 0 {
		if r.ContentLength > retainedBytes {
			retainedBytes = 0
		} else {
			retainedBytes = r.ContentLength
		}
	}
	return service.transportBudget.Reserve(transportbody.TransformationWorkingSet + retainedBytes)
}

func (service *Service) prepareOrHandleTransport(w http.ResponseWriter, r *http.Request, workingSet *transportbody.Reservation) bool {
	if !transportInferenceRequest(r) {
		return false
	}
	limits := service.transportLimits
	if r.ContentLength > limits.MaxRequestBytes {
		writeTransportError(w, transportbody.ErrRequestTooLarge)
		return true
	}
	externalSelector := transportExternalSelector(r)
	threshold := limits.ReplayBufferBytes
	if limits.SelectorScanBytes < threshold {
		threshold = limits.SelectorScanBytes
	}
	if r.ContentLength >= 0 && r.ContentLength <= threshold {
		reservation, ok := service.transportBudget.Reserve(r.ContentLength)
		if !ok {
			writeTransportError(w, transportbody.ErrBufferCapacity)
			return true
		}
		r.Body = &releaseReadCloser{ReadCloser: r.Body, release: reservation.Release}
		return false
	}

	body, err := transportbody.Prepare(r.Body, r.ContentLength, externalSelector != "", limits, service.transportBudget)
	if err != nil {
		writeTransportError(w, err)
		return true
	}
	if body.Replayable() {
		size, _ := body.Size()
		workingSet.ShrinkTo(transportbody.TransformationWorkingSet + size)
		return service.restoreReplayableRequest(w, r, body)
	}
	workingSet.ShrinkTo(transportbody.TransformationWorkingSet)
	defer body.Close()
	service.handleStreamingRequest(w, r, body, externalSelector)
	return true
}

func (service *Service) restoreReplayableRequest(w http.ResponseWriter, r *http.Request, body transportbody.Body) bool {
	size, _ := body.Size()
	attempt, err := body.OpenAttempt()
	if err != nil {
		_ = body.Close()
		writeTransportError(w, err)
		return true
	}
	r.Body = &releaseReadCloser{
		ReadCloser: attempt,
		release: func() {
			_ = body.Close()
		},
	}
	r.ContentLength = size
	return false
}

func transportInferenceRequest(r *http.Request) bool {
	if r == nil || r.Body == nil {
		return false
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return false
	}
	if inferenceControlPath(r.URL.Path) {
		return false
	}
	return isTextPath(r.URL.Path) || isImagePath(r.URL.Path) || isVoicePath(r.URL.Path) || isMusicPath(r.URL.Path)
}

func inferenceControlPath(path string) bool {
	return path == "/sdapi/v1/options" || path == "/sdapi/v1/refresh-checkpoints"
}

func transportExternalSelector(r *http.Request) string {
	for _, key := range []string{"model", "sd_model_checkpoint"} {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(r.Header.Get("X-Tensors-Model"))
}

func transportRequestIsJSON(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.Contains(strings.ToLower(mediaType), "json")
}

func writeTransportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, transportbody.ErrRequestTooLarge):
		openai.WriteError(w, http.StatusRequestEntityTooLarge, "request_too_large", transportbody.ErrRequestTooLarge.Error())
	case errors.Is(err, transportbody.ErrSelectorRequired):
		openai.WriteError(w, http.StatusBadRequest, "streaming_model_selector_required", transportbody.ErrSelectorRequired.Error())
	case errors.Is(err, transportbody.ErrBufferCapacity):
		openai.WriteError(w, http.StatusServiceUnavailable, "buffer_capacity_exceeded", transportbody.ErrBufferCapacity.Error())
	case errors.Is(err, transportbody.ErrResponseTooLarge):
		openai.WriteError(w, http.StatusBadGateway, "response_too_large", transportbody.ErrResponseTooLarge.Error())
	default:
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
}
