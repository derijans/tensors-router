package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	routeranalytics "tensors-router/internal/analytics"
	"time"
)

type backendRetryResult struct {
	retry    bool
	inactive bool
	status   int
	err      error
	body     string
	// emptyOutput marks a successful response whose only defect is that it carried no
	// generated text. It is retried in case the backend was still warming, but it stays
	// serviceable: if the retries run out it is returned to the client as-is.
	emptyOutput bool
	header      http.Header
}

func (service *Service) forwardWithFallback(ctx context.Context, original *http.Request, body []byte, modelID string, configFilename string, hasModel bool, readiness backendReadiness, mode string) (*http.Response, error) {
	response, _, err := service.forwardWithFallbackObserved(ctx, original, body, modelID, configFilename, hasModel, readiness, mode)
	return response, err
}

func (service *Service) forwardWithFallbackObserved(ctx context.Context, original *http.Request, body []byte, modelID string, configFilename string, hasModel bool, readiness backendReadiness, mode string) (*http.Response, routeranalytics.EventFinalizer, error) {
	if _, err := service.runtimeForBackendMode(mode, readiness); err != nil {
		return nil, nil, err
	}
	var runtime *backendRuntime
	var err error
	loadedFresh := false
	modelContext := ctx
	releaseModel := func() {}
	var workFinalizer routeranalytics.EventFinalizer
	if hasModel {
		var cancelModelContext context.CancelFunc
		modelContext, cancelModelContext = context.WithTimeout(context.WithoutCancel(ctx), modelOperationTimeout)
		defer cancelModelContext()

		var acquireErr error
		runtime, releaseModel, loadedFresh, acquireErr = service.acquireModelConfigForBackendMode(mode, modelContext, modelID, configFilename, readiness, false)
		if acquireErr != nil {
			return nil, nil, acquireErr
		}
		workFinalizer = service.analytics.beginWork(runtime)
	} else {
		if err := service.ensureBackendFamily(ctx, mode); err != nil {
			return nil, nil, err
		}
		runtime, err = service.runtimeForBackendMode(mode, readiness)
		if err != nil {
			return nil, nil, err
		}
	}

	response, err := service.forward(runtime, ctx, original, body)
	if !hasModel {
		return response, stampLoadedModel(runtime), err
	}
	recoveredBackend := false
	retryResult := service.backendRetryResult(response, err, original.URL.Path)
	if !retryResult.retry {
		return responseWithRelease(response, releaseModel), workFinalizer, nil
	}

	lastStatus := retryResult.status
	lastErr := retryResult.err
	lastBody := retryResult.body
	service.logRetryableBackendResult(original.URL.Path, modelID, configFilename, 0, lastStatus, lastErr, lastBody)

	if service.shouldRecoverBackend(runtime, ctx, retryResult) {
		var recoveryErr error
		releaseModel, recoveryErr = service.recoverBackendForModel(runtime, modelContext, releaseModel, modelID, configFilename, readiness, original.URL.Path, retryResult.err)
		if recoveryErr != nil {
			return nil, workFinalizer, recoveryErr
		}
		recoveredBackend = true
	} else if !loadedFresh && !isBackendWaitingResponse(lastStatus, lastBody) {
		// Retry against the runtime we already hold. Reloading here used to mean
		// releasing the lease first, which opened a window for a request wanting a
		// different model to take the runtime and switch it. The retry then had to switch
		// it back, and under two competing models the pair could trade the runtime until
		// the budget ran out — which is how a long request, in flight long enough to
		// overlap someone else's switch, ends as a 502. The backend being genuinely
		// unhealthy is handled above, where a reload is actually warranted.
		service.logger.Printf("backend returned a retryable response; retrying without reload model=%q config=%q status=%d", modelID, configFilename, lastStatus)
	} else if !loadedFresh {
		service.logger.Printf("backend unavailable while config already active; retrying without reload model=%q config=%q", modelID, configFilename)
	} else {
		service.logger.Printf("backend retry after fresh config load model=%q config=%q", modelID, configFilename)
	}

	retryDelay := service.backendRetryDelay
	skipRetryDelay := false
	if retryResult.inactive {
		if err := service.waitForInactiveBackend(runtime, modelContext, readiness, modelID, configFilename, original.URL.Path); err != nil {
			releaseModel()
			return nil, workFinalizer, err
		}
		skipRetryDelay = true
	}
	for attempt := 1; attempt <= service.inferenceRetryAttempts(retryResult); attempt++ {
		if attempt > 1 && !skipRetryDelay {
			if err := waitForRetry(ctx, retryDelay); err != nil {
				releaseModel()
				return nil, workFinalizer, err
			}
			retryDelay = nextRetryDelay(retryDelay, service.backendRetryMaxDelay)
		}
		skipRetryDelay = false

		response, err = service.forward(runtime, ctx, original, body)
		retryResult = service.backendRetryResult(response, err, original.URL.Path)
		if !retryResult.retry {
			return responseWithRelease(response, releaseModel), workFinalizer, nil
		}

		lastStatus = retryResult.status
		lastErr = retryResult.err
		lastBody = retryResult.body
		service.logRetryableBackendResult(original.URL.Path, modelID, configFilename, attempt, lastStatus, lastErr, lastBody)

		if !recoveredBackend && service.shouldRecoverBackend(runtime, ctx, retryResult) {
			var recoveryErr error
			releaseModel, recoveryErr = service.recoverBackendForModel(runtime, modelContext, releaseModel, modelID, configFilename, readiness, original.URL.Path, retryResult.err)
			if recoveryErr != nil {
				return nil, workFinalizer, recoveryErr
			}
			recoveredBackend = true
		}
		if retryResult.inactive {
			if err := service.waitForInactiveBackend(runtime, modelContext, readiness, modelID, configFilename, original.URL.Path); err != nil {
				releaseModel()
				return nil, workFinalizer, err
			}
			skipRetryDelay = true
		}
	}

	service.logger.Printf("backend retry exhausted path=%s model=%q config=%q attempts=%d status=%d error=%v body=%q", original.URL.Path, modelID, configFilename, service.inferenceRetryAttempts(retryResult), lastStatus, lastErr, lastBody)
	// The backend never produced generated text, but it did answer successfully every
	// time. That is a valid — if empty — completion, so return it instead of turning it
	// into a gateway error the client cannot act on.
	if retryResult.emptyOutput {
		return responseWithRelease(emptyOutputResponse(retryResult), releaseModel), workFinalizer, nil
	}
	releaseModel()
	return nil, workFinalizer, backendRetryExhaustedError(lastStatus, lastErr, lastBody)
}

// inferenceRetryAttempts sizes the retry budget for a request that is being re-run. A
// backend still reporting itself inactive is waited out with the large readiness budget;
// anything else gets the small inference budget, because each attempt regenerates.
func (service *Service) inferenceRetryAttempts(result backendRetryResult) int {
	if result.inactive {
		return service.backendRetryAttempts
	}
	attempts := service.backendInferenceRetryAttempts
	if attempts < 1 {
		attempts = defaultBackendInferenceRetryAttempts
	}
	if attempts > service.backendRetryAttempts {
		attempts = service.backendRetryAttempts
	}
	return attempts
}

func emptyOutputResponse(result backendRetryResult) *http.Response {
	header := result.header
	if header == nil {
		header = http.Header{}
		header.Set("Content-Type", "application/json")
	}
	status := result.status
	if status == 0 {
		status = http.StatusOK
	}
	body := []byte(result.body)
	header = header.Clone()
	header.Del("Content-Length")
	return &http.Response{
		StatusCode:    status,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func (service *Service) shouldRecoverBackend(runtime *backendRuntime, ctx context.Context, retryResult backendRetryResult) bool {
	if retryResult.err == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return !runtime.backend.Healthy(ctx)
}

func (service *Service) recoverBackendForModel(runtime *backendRuntime, ctx context.Context, releaseModel func(), modelID string, configFilename string, readiness backendReadiness, path string, cause error) (func(), error) {
	service.logger.Printf("backend transport recovery attempt path=%s model=%q config=%q error=%v", path, modelID, configFilename, cause)
	// Keep the lease. Releasing it here let a request for another model take the runtime
	// mid-recovery and switch it away, which is how the model this request needs ended up
	// being swapped out from under it.
	if err := service.reloadHeldModelConfig(runtime, ctx, modelID, configFilename, readiness); err != nil {
		releaseModel()
		return nil, err
	}
	service.logger.Printf("backend transport recovery succeeded path=%s model=%q config=%q reloaded=true", path, modelID, configFilename)
	return releaseModel, nil
}

func (service *Service) waitForInactiveBackend(runtime *backendRuntime, ctx context.Context, readiness backendReadiness, modelID string, configFilename string, path string) error {
	service.logger.Printf("backend inactive response path=%s model=%q config=%q", path, modelID, configFilename)
	return service.waitForBackendEndpoint(runtime, ctx, readiness, modelID, configFilename)
}

func (service *Service) backendRetryResult(response *http.Response, err error, path string) backendRetryResult {
	if err != nil {
		return backendRetryResult{retry: true, err: err}
	}
	if response == nil {
		return backendRetryResult{retry: true, err: errMissingBackendResponse}
	}
	if response.Body == nil {
		return backendRetryResult{retry: true, status: response.StatusCode, err: errMissingBackendResponse}
	}
	if response.StatusCode >= 500 {
		return backendRetryResult{
			retry:  true,
			status: response.StatusCode,
			body:   drainResponseBodyPreview(response),
		}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return service.successRetryResult(response, path)
	}
	return backendRetryResult{status: response.StatusCode}
}

func (service *Service) successRetryResult(response *http.Response, path string) backendRetryResult {
	if !isCorePath(path) {
		return backendRetryResult{status: response.StatusCode}
	}
	if response.Body == nil {
		return backendRetryResult{retry: true, status: response.StatusCode}
	}
	if isEventStream(response.Header) {
		buffer := make([]byte, 1)
		read, err := response.Body.Read(buffer)
		if read > 0 {
			response.Body = replayReadCloser{
				Reader: io.MultiReader(bytes.NewReader(buffer[:read]), response.Body),
				closer: response.Body,
			}
			return backendRetryResult{status: response.StatusCode}
		}
		_ = response.Body.Close()
		if err == io.EOF {
			return backendRetryResult{retry: true, status: response.StatusCode}
		}
		return backendRetryResult{retry: true, status: response.StatusCode, err: err}
	}

	if response.ContentLength > backendResponseMetadataLimit {
		return backendRetryResult{status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, backendResponseMetadataLimit+1))
	if err != nil {
		_ = response.Body.Close()
		return backendRetryResult{retry: true, status: response.StatusCode, err: err}
	}
	if len(body) > backendResponseMetadataLimit {
		response.Body = replayReadCloser{
			Reader: io.MultiReader(bytes.NewReader(body), response.Body),
			closer: response.Body,
		}
		return backendRetryResult{status: response.StatusCode}
	}
	_ = response.Body.Close()
	if len(bytes.TrimSpace(body)) == 0 {
		return backendRetryResult{retry: true, status: response.StatusCode}
	}
	if isCorePath(path) && isJSONResponse(response.Header) {
		inactive := coreResponseIsInactive(body)
		if inactive || coreResponseHasEmptyText(body) {
			// An empty completion usually means the backend is still warming, so it is
			// worth retrying — but it can also be a genuine empty answer. Carry the body
			// so that if the retries run out we can hand the client this response rather
			// than inventing a gateway error out of a valid 2xx.
			return backendRetryResult{
				retry:       true,
				inactive:    inactive,
				status:      response.StatusCode,
				body:        strings.TrimSpace(string(body)),
				emptyOutput: !inactive,
				header:      response.Header.Clone(),
			}
		}
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return backendRetryResult{status: response.StatusCode}
}

func coreResponseIsInactive(body []byte) bool {
	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(parsed.Model), "inactive") {
		return true
	}
	if !coreResponseHasEmptyText(body) {
		return false
	}
	for _, choice := range parsed.Choices {
		if strings.EqualFold(strings.TrimSpace(choice.FinishReason), "error") {
			return true
		}
	}
	return false
}

func coreResponseHasEmptyText(body []byte) bool {
	var parsed struct {
		Choices []json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	if parsed.Choices == nil {
		return false
	}
	if len(parsed.Choices) == 0 {
		return true
	}

	foundOutput := false
	for _, choice := range parsed.Choices {
		if choiceHasToolOutput(choice) {
			return false
		}
		output, ok := choiceOutputText(choice)
		if !ok {
			continue
		}
		foundOutput = true
		if strings.TrimSpace(output) != "" {
			return false
		}
	}
	return foundOutput
}

func choiceHasToolOutput(choice json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(choice, &fields); err != nil {
		return false
	}
	for _, key := range []string{"message", "delta"} {
		message, ok := fields[key]
		if ok && messageHasToolOutput(message) {
			return true
		}
	}
	return false
}

func messageHasToolOutput(message json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(message, &fields); err != nil {
		return false
	}
	if rawCalls, ok := fields["tool_calls"]; ok {
		var calls []json.RawMessage
		if json.Unmarshal(rawCalls, &calls) == nil && len(calls) > 0 {
			return true
		}
	}
	if rawCall, ok := fields["function_call"]; ok {
		var call map[string]json.RawMessage
		if json.Unmarshal(rawCall, &call) == nil && len(call) > 0 {
			return true
		}
	}
	return false
}

func choiceOutputText(choice json.RawMessage) (string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(choice, &fields); err != nil {
		return "", false
	}
	if text, ok := jsonStringField(fields, "text"); ok {
		return text, true
	}
	message, ok := fields["message"]
	if ok {
		if text, ok := messageContentText(message); ok {
			return text, true
		}
	}
	delta, ok := fields["delta"]
	if ok {
		if text, ok := messageContentText(delta); ok {
			return text, true
		}
	}
	return "", false
}

func messageContentText(message json.RawMessage) (string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(message, &fields); err != nil {
		return "", false
	}
	// A thinking model may answer with only reasoning_content/reasoning and an empty
	// content field; that is a complete response, not evidence the backend is still
	// warming up, so treat non-empty reasoning text as output too.
	for _, key := range []string{"content", "reasoning_content", "reasoning"} {
		if text, ok := jsonStringField(fields, key); ok && strings.TrimSpace(text) != "" {
			return text, true
		}
	}
	return jsonStringField(fields, "content")
}

func jsonStringField(fields map[string]json.RawMessage, key string) (string, bool) {
	value, ok := fields[key]
	if !ok {
		return "", false
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", false
	}
	return text, true
}

func isKoboldUnavailableResponse(status int, body string) bool {
	return status == http.StatusBadGateway && strings.Contains(body, "KoboldCpp is not available.")
}

func isBackendWaitingResponse(status int, body string) bool {
	if isKoboldUnavailableResponse(status, body) {
		return true
	}
	if status < 200 || status >= 300 {
		return false
	}
	return strings.TrimSpace(body) == "" || coreResponseIsInactive([]byte(body)) || coreResponseHasEmptyText([]byte(body))
}

func (service *Service) logRetryableBackendResult(path string, modelID string, configFilename string, attempt int, status int, err error, body string) {
	if attempt > 0 && attempt != 1 && attempt%30 != 0 {
		return
	}
	if err != nil {
		if attempt == 0 {
			service.logger.Printf("backend request failed path=%s model=%q config=%q error=%v", path, modelID, configFilename, err)
			return
		}
		service.logger.Printf("backend retry failed path=%s model=%q config=%q attempt=%d error=%v", path, modelID, configFilename, attempt, err)
		return
	}
	if attempt == 0 {
		service.logger.Printf("backend returned retryable status path=%s model=%q config=%q status=%d body=%q", path, modelID, configFilename, status, body)
		return
	}
	service.logger.Printf("backend retry returned retryable status path=%s model=%q config=%q attempt=%d status=%d body=%q", path, modelID, configFilename, attempt, status, body)
}

func drainResponseBodyPreview(response *http.Response) string {
	if response == nil || response.Body == nil {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, backendErrorBodyLimit+1))
	_ = response.Body.Close()
	if len(body) > backendErrorBodyLimit {
		body = body[:backendErrorBodyLimit]
	}
	return strings.TrimSpace(string(body))
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextRetryDelay(delay time.Duration, maxDelay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}
	next := delay * 2
	if maxDelay > 0 && next > maxDelay {
		return maxDelay
	}
	return next
}

func backendRetryExhaustedError(status int, err error, body string) error {
	if err != nil {
		return fmt.Errorf("backend unavailable after retries: %w", err)
	}
	if status > 0 && body != "" {
		return fmt.Errorf("backend unavailable after retries: status %d: %s", status, body)
	}
	if status > 0 {
		return fmt.Errorf("backend unavailable after retries: status %d", status)
	}
	return fmt.Errorf("backend unavailable after retries")
}

func (service *Service) probeBackendEndpoint(runtime *backendRuntime, ctx context.Context, path string) (int, string, error) {
	target := runtime.backend.URL()
	target.Path = joinPath(target.Path, path)
	target.RawQuery = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, "", err
	}

	response, err := service.backendHTTPClient(runtime.backend).Do(request)
	if err != nil {
		return 0, "", err
	}
	return response.StatusCode, drainResponseBodyPreview(response), nil
}

func (service *Service) forward(runtime *backendRuntime, ctx context.Context, original *http.Request, body []byte) (*http.Response, error) {
	target := runtime.backend.URL()
	responseFormat := ""
	if runtime.mode == BackendModeLlamaSDCPP && strings.HasSuffix(runtime.name, "-transcription") && isVoicePath(original.URL.Path) {
		responseFormat = original.Header.Get("X-Tensors-Whisper-Response-Format")
		target.Path = joinPath(target.Path, "/inference")
	} else {
		target.Path = joinPath(target.Path, original.URL.Path)
	}
	target.RawQuery = original.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	copyBackendHeaders(request.Header, original.Header)
	request.Header.Del("X-Tensors-Whisper-Response-Format")
	request.Host = target.Host

	response, err := service.backendHTTPClient(runtime.backend).Do(request)
	if err != nil || responseFormat == "" {
		return response, err
	}
	return adaptWhisperResponse(response, responseFormat)
}

func (service *Service) httpClient() *http.Client {
	return service.client
}

func (service *Service) backendHTTPClient(backend Backend) *http.Client {
	if provider, ok := backend.(backendHTTPClientProvider); ok {
		if client := provider.HTTPClient(); client != nil {
			return client
		}
	}
	return service.client
}
