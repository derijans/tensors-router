package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/openai"
	"tensors-router/internal/transportbody"
)

type nonReplayableTransportError struct {
	cause error
}

func (err nonReplayableTransportError) Error() string {
	return fmt.Sprintf("streaming request cannot be retried after outbound body consumption: %v; retry with the original client stream", err.cause)
}

func (err nonReplayableTransportError) Unwrap() error {
	return err.cause
}

func (service *Service) forwardTransportRoute(r *http.Request, body transportbody.Body, route transportRoute) (*http.Response, routeranalytics.Event, routeranalytics.EventFinalizer, error) {
	if route.remote {
		response, err := service.forwardTransportRemote(r.Context(), r, body, route.nodeURL)
		return response, routeranalytics.Event{}, nil, err
	}
	if err := service.prepareTransportCompanionRuntime(r.Context(), r.URL.Path, route); err != nil {
		return nil, routeranalytics.Event{}, nil, err
	}
	if route.passthrough {
		runtime, err := service.runtimeForBackendMode(route.backendMode, route.readiness)
		if err != nil {
			return nil, routeranalytics.Event{}, nil, err
		}
		response, err := service.forwardTransportLocal(runtime, r.Context(), r, body)
		return response, routeranalytics.Event{}, nil, err
	}
	modelContext, cancel := context.WithTimeout(r.Context(), modelOperationTimeout)
	defer cancel()
	runtime, release, _, err := service.acquireModelConfigForBackendMode(route.backendMode, modelContext, route.publicID, route.configFilename, route.readiness, false)
	if err != nil {
		return nil, routeranalytics.Event{}, nil, err
	}
	finalizer := service.analytics.beginWork(runtime)
	event := service.analytics.newEvent(time.Now(), r, nil, route.localID, route.section, route.backendMode)
	response, err := service.forwardTransportLocal(runtime, r.Context(), r, body)
	if err != nil {
		release()
		return nil, event, finalizer, err
	}
	return responseWithRelease(response, release), event, finalizer, nil
}

func (service *Service) prepareTransportCompanionRuntime(ctx context.Context, path string, route transportRoute) error {
	if route.backendMode != BackendModeLlamaSDCPP {
		return nil
	}
	model := route.catalogModel
	hasLLM := model.HasLLM || route.clusterModel.HasLLM
	hasEmbeddings := model.HasEmbeddings || route.clusterModel.HasEmbeddings
	hasMultimodal := model.HasMultimodal || route.clusterModel.HasMultimodal
	hasImage := model.HasImage || route.clusterModel.HasImage
	textID := model.ID
	imageID := model.ImageID
	if textID == "" {
		textID = route.clusterModel.LocalID
	}
	if imageID == "" {
		imageID = route.clusterModel.ImageID
	}
	separateEmbeddings := (model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate) ||
		(route.clusterModel.Capabilities.Embeddings != nil && route.clusterModel.Capabilities.Embeddings.Separate)
	if route.readiness == readinessImage && (hasLLM || hasMultimodal || (hasEmbeddings && !separateEmbeddings)) {
		return service.loadLocalRuntimeForRequest(ctx, route.backendMode, textID, route.configFilename, readinessText)
	}
	if route.readiness == readinessText && hasImage && !isEmbeddingsPath(path) {
		return service.loadLocalRuntimeForRequest(ctx, route.backendMode, imageID, route.configFilename, readinessImage)
	}
	if isVoicePath(path) || isMusicPath(path) {
		if isMusicPath(path) || !routeSupportsSplitAudio(route, path) {
			return fmt.Errorf("audio route is not supported by the selected split backend config")
		}
	}
	return nil
}

func routeSupportsSplitAudio(route transportRoute, path string) bool {
	model := route.catalogModel
	if model.Capabilities.Voice == nil && route.clusterModel.Capabilities.Voice != nil {
		model.Capabilities.Voice = route.clusterModel.Capabilities.Voice
	}
	return modelSupportsLlamaAudioPath(model, path)
}

func (service *Service) forwardTransportLocal(runtime *backendRuntime, ctx context.Context, original *http.Request, body transportbody.Body) (*http.Response, error) {
	target := runtime.backend.URL()
	request := original
	responseFormat := ""
	if runtime.mode == BackendModeLlamaSDCPP && strings.HasSuffix(runtime.name, "-transcription") && isVoicePath(original.URL.Path) {
		request = original.Clone(original.Context())
		request.Header = original.Header.Clone()
		responseFormat = request.Header.Get("X-Tensors-Whisper-Response-Format")
		request.Header.Del("X-Tensors-Whisper-Response-Format")
		target.Path = joinPath(target.Path, "/inference")
	} else {
		target.Path = joinPath(target.Path, original.URL.Path)
	}
	target.RawQuery = original.URL.RawQuery
	response, err := service.doTransportAttempts(ctx, request, target, body, runtime.backend, false)
	if err != nil || responseFormat == "" {
		return response, err
	}
	return adaptWhisperResponse(response, responseFormat)
}

func (service *Service) forwardTransportRemote(ctx context.Context, original *http.Request, body transportbody.Body, nodeURL string) (*http.Response, error) {
	baseURL, err := service.clusterClient.AuthorizedBaseURL(nodeURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	target.Path = joinPath(target.Path, "/router/v1/node/inference"+original.URL.Path)
	target.RawQuery = original.URL.RawQuery
	return service.doTransportAttempts(ctx, original, target, body, nil, true)
}

func (service *Service) doTransportAttempts(ctx context.Context, original *http.Request, target *url.URL, body transportbody.Body, backend Backend, clusterRequest bool) (*http.Response, error) {
	response, consumed, err := service.doTransportAttempt(ctx, original, target, body, backend, clusterRequest)
	if !transportAttemptFailed(response, err) {
		return response, nil
	}
	if consumed == 0 && body.CanRetry() && !errors.Is(err, transportbody.ErrRequestTooLarge) {
		closeTransportResponse(response)
		response, consumed, err = service.doTransportAttempt(ctx, original, target, body, backend, clusterRequest)
		if !transportAttemptFailed(response, err) {
			return response, nil
		}
	}
	return nil, finalTransportFailure(response, err, consumed)
}

func finalTransportFailure(response *http.Response, err error, consumed int64) error {
	if err == nil && response != nil && response.Body != nil {
		status := response.StatusCode
		return backendStatusError(status, drainResponseBodyPreview(response))
	}
	closeTransportResponse(response)
	cause := transportAttemptCause(response, err)
	if errors.Is(cause, transportbody.ErrRequestTooLarge) {
		return transportbody.ErrRequestTooLarge
	}
	if consumed > 0 {
		return nonReplayableTransportError{cause: cause}
	}
	return cause
}

func backendStatusError(status int, bodyPreview string) error {
	if bodyPreview == "" {
		return fmt.Errorf("backend returned status %d", status)
	}
	return fmt.Errorf("backend returned status %d: %s", status, bodyPreview)
}

func (service *Service) doTransportAttempt(ctx context.Context, original *http.Request, target *url.URL, body transportbody.Body, backend Backend, clusterRequest bool) (*http.Response, int64, error) {
	attempt, err := body.OpenAttempt()
	if err != nil {
		return nil, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), attempt)
	if err != nil {
		_ = attempt.Close()
		return nil, 0, err
	}
	if clusterRequest {
		copyClusterRequestHeaders(request.Header, original.Header)
		request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	} else {
		copyBackendHeaders(request.Header, original.Header)
	}
	if size, known := body.Size(); known {
		request.ContentLength = size
	}
	request.Host = target.Host
	response, requestErr := service.backendHTTPClient(backend).Do(request)
	consumed := attempt.BytesRead()
	_ = attempt.Close()
	return response, consumed, requestErr
}

func transportAttemptFailed(response *http.Response, err error) bool {
	return err != nil || response == nil || response.StatusCode >= http.StatusInternalServerError
}

func transportAttemptCause(response *http.Response, err error) error {
	if err != nil {
		return err
	}
	if response == nil {
		return errMissingBackendResponse
	}
	return fmt.Errorf("backend returned status %d", response.StatusCode)
}

func closeTransportResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func writeTransportForwardError(w http.ResponseWriter, err error) {
	var nonReplayable nonReplayableTransportError
	switch {
	case errors.Is(err, transportbody.ErrRequestTooLarge):
		writeTransportError(w, transportbody.ErrRequestTooLarge)
	case errors.As(err, &nonReplayable):
		openai.WriteError(w, http.StatusBadGateway, "non_replayable_transport_error", err.Error())
	default:
		writeBackendFailure(w, err)
	}
}
