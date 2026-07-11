package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func (service *Service) forwardTransportRoute(r *http.Request, body transportbody.Body, route transportRoute) (*http.Response, routeranalytics.Event, analyticsEventFinalizer, error) {
	if route.remote {
		response, err := service.forwardTransportRemote(r.Context(), r, body, route.nodeURL)
		return response, routeranalytics.Event{}, nil, err
	}
	if err := service.prepareTransportCompanionRuntime(r.Context(), r.URL.Path, route); err != nil {
		return nil, routeranalytics.Event{}, nil, err
	}
	modelContext, cancel := context.WithTimeout(r.Context(), modelOperationTimeout)
	defer cancel()
	runtime, release, _, err := service.acquireModelConfigForBackendMode(route.backendMode, modelContext, route.publicID, route.configFilename, route.readiness, false)
	if err != nil {
		return nil, routeranalytics.Event{}, nil, err
	}
	finalizer := service.beginVRAMWork(runtime)
	event := service.newAnalyticsEvent(time.Now(), r, nil, route.localID, route.section, route.backendMode)
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
	if route.readiness == readinessImage && (hasLLM || hasEmbeddings || hasMultimodal) {
		return service.loadLocalRuntimeForRequest(ctx, route.backendMode, textID, route.configFilename, readinessText)
	}
	if route.readiness == readinessText && hasImage && !isEmbeddingsPath(path) {
		return service.loadLocalRuntimeForRequest(ctx, route.backendMode, imageID, route.configFilename, readinessImage)
	}
	if isVoicePath(path) || isMusicPath(path) {
		if isMusicPath(path) || !modelSupportsLlamaAudioPath(model, path) {
			return fmt.Errorf("audio route is not supported by the selected split backend config")
		}
	}
	return nil
}

func (service *Service) forwardTransportLocal(runtime *backendRuntime, ctx context.Context, original *http.Request, body transportbody.Body) (*http.Response, error) {
	target := runtime.backend.URL()
	target.Path = joinPath(target.Path, original.URL.Path)
	target.RawQuery = original.URL.RawQuery
	return service.doTransportAttempts(ctx, original, target, body, false)
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
	return service.doTransportAttempts(ctx, original, target, body, true)
}

func (service *Service) doTransportAttempts(ctx context.Context, original *http.Request, target *url.URL, body transportbody.Body, clusterRequest bool) (*http.Response, error) {
	response, consumed, err := service.doTransportAttempt(ctx, original, target, body, clusterRequest)
	if !transportAttemptFailed(response, err) {
		return response, nil
	}
	cause := transportAttemptCause(response, err)
	if errors.Is(cause, transportbody.ErrRequestTooLarge) {
		closeTransportResponse(response)
		return nil, transportbody.ErrRequestTooLarge
	}
	if consumed > 0 {
		closeTransportResponse(response)
		return nil, nonReplayableTransportError{cause: cause}
	}
	closeTransportResponse(response)
	if !body.CanRetry() {
		return nil, cause
	}
	response, consumed, err = service.doTransportAttempt(ctx, original, target, body, clusterRequest)
	if !transportAttemptFailed(response, err) {
		return response, nil
	}
	cause = transportAttemptCause(response, err)
	closeTransportResponse(response)
	if consumed > 0 {
		return nil, nonReplayableTransportError{cause: cause}
	}
	return nil, cause
}

func (service *Service) doTransportAttempt(ctx context.Context, original *http.Request, target *url.URL, body transportbody.Body, clusterRequest bool) (*http.Response, int64, error) {
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
	client := *service.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, requestErr := client.Do(request)
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

func (service *Service) writeTransportResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error {
	if response == nil || response.Body == nil {
		return writeMissingBackendResponse(w)
	}
	defer response.Body.Close()
	if response.ContentLength > service.transportLimits.MaxResponseBytes {
		writeTransportError(w, transportbody.ErrResponseTooLarge)
		return nil
	}
	response.Body = &readerReadCloser{
		Reader: transportbody.LimitResponse(response.Body, service.transportLimits.MaxResponseBytes),
		Closer: response.Body,
	}
	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isEventStream(response.Header) {
		return writeEventStreamResponse(w, response, virtualModelID)
	}
	source := io.ReadCloser(response.Body)
	transformed := false
	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isJSONResponse(response.Header) {
		source = transportbody.NewJSONTransformReadCloser(response.Body, transportbody.JSONRewrite{
			Replacements: map[string]transportbody.StringReplacement{
				transportbody.PathModel: {To: virtualModelID},
			},
			EscapeHTML: true,
		})
		transformed = true
		defer source.Close()
	}
	copyResponseHeaders(w.Header(), response.Header)
	if transformed {
		w.Header().Del("Content-Length")
	}
	w.WriteHeader(response.StatusCode)
	_, err := transportbody.CopyResponse(flushingWriter{ResponseWriter: w}, source, service.transportLimits.MaxResponseBytes)
	return err
}

func writeTransportForwardError(w http.ResponseWriter, err error) {
	var nonReplayable nonReplayableTransportError
	switch {
	case errors.Is(err, transportbody.ErrRequestTooLarge):
		writeTransportError(w, transportbody.ErrRequestTooLarge)
	case errors.As(err, &nonReplayable):
		openai.WriteError(w, http.StatusBadGateway, "non_replayable_transport_error", err.Error())
	default:
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
	}
}

type readerReadCloser struct {
	io.Reader
	Closer io.Closer
}

func (reader *readerReadCloser) Close() error {
	return reader.Closer.Close()
}

type flushingWriter struct {
	http.ResponseWriter
}

func (writer flushingWriter) Write(content []byte) (int, error) {
	written, err := writer.ResponseWriter.Write(content)
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return written, err
}
