package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/openai"
)

type Backend interface {
	URL() *url.URL
	ReloadConfig(ctx context.Context, filename string) error
	Restart(ctx context.Context) error
	Healthy(ctx context.Context) bool
}

type ModelCatalog interface {
	List() ([]catalog.Model, error)
	ListLLM() ([]catalog.Model, error)
	ListImages(activeConfigFilename string) ([]catalog.Model, error)
	Resolve(id string) (catalog.Model, bool, error)
	ResolveImage(id string, activeConfigFilename string) (catalog.Model, bool, error)
	ResolveActiveImage(activeConfigFilename string) (catalog.Model, bool, error)
}

type ServiceConfig struct {
	Backend Backend
	Catalog ModelCatalog
	Logger  *log.Logger
}

type Service struct {
	backend Backend
	catalog ModelCatalog
	client  *http.Client
	logger  *log.Logger

	activeConfigMu            sync.Mutex
	activeConfigChanged       chan struct{}
	activeConfigFilename      string
	activeConfigUsers         int
	activeConfigSwitching     bool
	activeConfigSwitchWaiters int

	backendRetryAttempts int
	backendRetryDelay    time.Duration
	backendRetryMaxDelay time.Duration
}

const (
	defaultBackendRetryAttempts = 300
	defaultBackendRetryDelay    = 1 * time.Second
	defaultBackendRetryMaxDelay = 2 * time.Second
	backendErrorBodyLimit       = 1024
	modelOperationTimeout       = 15 * time.Minute
)

type backendReadiness string

const (
	readinessText  backendReadiness = "/v1/models"
	readinessImage backendReadiness = "/sdapi/v1/sd-models"
)

type backendRetryResult struct {
	retry    bool
	inactive bool
	status   int
	err      error
	body     string
}

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (body replayReadCloser) Close() error {
	return body.closer.Close()
}

type releaseReadCloser struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (body *releaseReadCloser) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(body.release)
	return err
}

type imageModelObject struct {
	Title     string `json:"title"`
	ModelName string `json:"model_name"`
	Hash      string `json:"hash"`
	SHA256    string `json:"sha256"`
	Filename  string `json:"filename"`
	Config    string `json:"config"`
}

func NewService(config ServiceConfig) *Service {
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Service{
		backend: config.Backend,
		catalog: config.Catalog,
		logger:  logger,
		client: &http.Client{
			Timeout: 0,
		},
		activeConfigChanged:  make(chan struct{}),
		backendRetryAttempts: defaultBackendRetryAttempts,
		backendRetryDelay:    defaultBackendRetryDelay,
		backendRetryMaxDelay: defaultBackendRetryMaxDelay,
	}
}

func (service *Service) PreloadModel(ctx context.Context, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	model, ok, err := service.catalog.Resolve(modelID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("startup model %q was not found", modelID)
	}
	if !model.HasLLM {
		return fmt.Errorf("startup model %q is not an LLM model", modelID)
	}

	modelContext, cancelModelContext := context.WithTimeout(ctx, modelOperationTimeout)
	defer cancelModelContext()

	release, _, err := service.acquireModelConfig(modelContext, model.ID, model.Filename, readinessText, false)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (service *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		service.handleModels(w)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
		service.handleImageModels(w)
		return
	}

	if r.URL.Path == "/sdapi/v1/options" {
		service.handleImageOptions(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/sdapi/v1/refresh-checkpoints" {
		openai.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}

	if r.Method == http.MethodPost && isCorePath(r.URL.Path) {
		service.handleModelRequest(w, r, true)
		return
	}

	if isImagePath(r.URL.Path) {
		service.handleImageRequest(w, r)
		return
	}

	if isOpenAIPath(r.URL.Path) {
		service.handleModelRequest(w, r, false)
		return
	}

	openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
}

func (service *Service) handleModels(w http.ResponseWriter) {
	models, err := service.catalog.ListLLM()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, openai.ModelsResponseFromCatalog(models))
}

func (service *Service) handleImageModels(w http.ResponseWriter) {
	models, err := service.catalog.ListImages(service.currentConfigFilename())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}

	response := make([]imageModelObject, 0, len(models))
	for _, model := range models {
		response = append(response, imageModelObject{
			Title:     model.ImageID,
			ModelName: model.ImageID,
			Hash:      "",
			SHA256:    "",
			Filename:  model.ImageModelPath,
			Config:    model.Filename,
		})
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleImageOptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		model, ok, err := service.catalog.ResolveActiveImage(service.currentConfigFilename())
		if err != nil {
			openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
			return
		}
		checkpoint := ""
		if ok {
			checkpoint = model.ImageID
		}
		openai.WriteJSON(w, http.StatusOK, map[string]any{
			"sd_model_checkpoint": checkpoint,
		})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
			return
		}
		defer r.Body.Close()

		modelID, hasModel, err := imageModelFromRequest(body, r)
		if err != nil {
			service.logger.Printf("image model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if hasModel {
			model, ok, err := service.catalog.ResolveImage(modelID, service.currentConfigFilename())
			if err != nil {
				service.logger.Printf("image model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
				openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
				return
			}
			if !ok {
				service.logger.Printf("unknown image model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
				openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("image model %q was not found", modelID))
				return
			}
			modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
			defer cancelModelContext()
			release, _, err := service.acquireModelConfig(modelContext, model.ImageID, model.Filename, readinessImage, false)
			if err != nil {
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return
			}
			release()
		}
		openai.WriteJSON(w, http.StatusOK, map[string]any{})
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) handleImageRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
		return
	}
	defer r.Body.Close()

	modelID, hasModel, err := imageModelFromRequest(body, r)
	if err != nil {
		service.logger.Printf("image model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	var model catalog.Model
	if hasModel {
		model, err = service.resolveImageModel(r, modelID)
		if err != nil {
			writeImageModelError(service, w, r, modelID, err)
			return
		}
	} else {
		model, err = service.activeImageModel(r)
		if err != nil {
			writeImageModelError(service, w, r, modelID, err)
			return
		}
		modelID = model.ImageID
		hasModel = true
	}

	response, err := service.forwardWithFallback(r.Context(), r, body, model.ImageID, model.Filename, hasModel, readinessImage)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}

	if err := writeProxyResponse(w, response, model.ImageID, hasModel); err != nil {
		return
	}
}

type imageModelLookupError struct {
	status  int
	message string
}

func (err imageModelLookupError) Error() string {
	return err.message
}

func (service *Service) resolveImageModel(r *http.Request, modelID string) (catalog.Model, error) {
	model, ok, err := service.catalog.ResolveImage(modelID, service.currentConfigFilename())
	if err != nil {
		service.logger.Printf("image model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
		return catalog.Model{}, err
	}
	if !ok {
		service.logger.Printf("unknown image model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		return catalog.Model{}, imageModelLookupError{
			status:  http.StatusNotFound,
			message: fmt.Sprintf("image model %q was not found", modelID),
		}
	}
	return model, nil
}

func (service *Service) activeImageModel(r *http.Request) (catalog.Model, error) {
	model, ok, err := service.catalog.ResolveActiveImage(service.currentConfigFilename())
	if err != nil {
		service.logger.Printf("active image model catalog check failed path=%s error=%v", r.URL.Path, err)
		return catalog.Model{}, err
	}
	if !ok {
		service.logger.Printf("image model missing path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		return catalog.Model{}, imageModelLookupError{
			status:  http.StatusBadRequest,
			message: "image model is required",
		}
	}
	return model, nil
}

func writeImageModelError(service *Service, w http.ResponseWriter, r *http.Request, modelID string, err error) {
	if lookupErr, ok := err.(imageModelLookupError); ok {
		openai.WriteError(w, lookupErr.status, "invalid_request_error", lookupErr.message)
		return
	}
	service.logger.Printf("image model lookup failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
	openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
}

func (service *Service) handleModelRequest(w http.ResponseWriter, r *http.Request, requireModel bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
		return
	}
	defer r.Body.Close()

	modelID, hasModel, err := modelFromRequest(body, r)
	if err != nil {
		service.logger.Printf("model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if requireModel && !hasModel {
		service.logger.Printf("model missing path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	configFilename := ""
	if hasModel {
		model, ok, err := service.catalog.Resolve(modelID)
		if err != nil {
			service.logger.Printf("model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
			openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
			return
		}
		if !ok {
			service.logger.Printf("unknown model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
			return
		}
		if !model.HasLLM {
			service.logger.Printf("non-llm model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
			return
		}
		configFilename = model.Filename
	}

	response, err := service.forwardWithFallback(r.Context(), r, body, modelID, configFilename, hasModel, readinessText)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}

	if err := writeProxyResponse(w, response, modelID, hasModel); err != nil {
		return
	}
}

func (service *Service) forwardWithFallback(ctx context.Context, original *http.Request, body []byte, modelID string, configFilename string, hasModel bool, readiness backendReadiness) (*http.Response, error) {
	loadedFresh := false
	modelContext := ctx
	cancelModelContext := func() {}
	releaseModel := func() {}
	if hasModel {
		modelContext, cancelModelContext = context.WithTimeout(context.WithoutCancel(ctx), modelOperationTimeout)
		defer cancelModelContext()

		var err error
		releaseModel, loadedFresh, err = service.acquireModelConfig(modelContext, modelID, configFilename, readiness, false)
		if err != nil {
			return nil, err
		}
	}

	response, err := service.forward(ctx, original, body)
	if !hasModel {
		return response, err
	}
	recoveredBackend := false
	retryResult := service.backendRetryResult(response, err, original.URL.Path)
	if !retryResult.retry {
		return responseWithRelease(response, releaseModel), nil
	}

	lastStatus := retryResult.status
	lastErr := retryResult.err
	lastBody := retryResult.body
	service.logRetryableBackendResult(original.URL.Path, modelID, configFilename, 0, lastStatus, lastErr, lastBody)

	if service.shouldRecoverBackend(ctx, retryResult) {
		var recoveryErr error
		releaseModel, loadedFresh, recoveryErr = service.recoverBackendForModel(modelContext, releaseModel, modelID, configFilename, readiness, original.URL.Path, retryResult.err)
		if recoveryErr != nil {
			return nil, recoveryErr
		}
		recoveredBackend = true
	} else if !loadedFresh && !isBackendWaitingResponse(lastStatus, lastBody) {
		releaseModel()
		var reloadErr error
		releaseModel, loadedFresh, reloadErr = service.acquireModelConfig(modelContext, modelID, configFilename, readiness, true)
		if reloadErr != nil {
			return nil, reloadErr
		}
	} else if !loadedFresh {
		service.logger.Printf("backend unavailable while config already active; retrying without reload model=%q config=%q", modelID, configFilename)
	} else {
		service.logger.Printf("backend retry after fresh config load model=%q config=%q", modelID, configFilename)
	}

	retryDelay := service.backendRetryDelay
	skipRetryDelay := false
	if retryResult.inactive {
		if err := service.waitForInactiveBackend(modelContext, readiness, modelID, configFilename, original.URL.Path); err != nil {
			releaseModel()
			return nil, err
		}
		skipRetryDelay = true
	}
	for attempt := 1; attempt <= service.backendRetryAttempts; attempt++ {
		if attempt > 1 && !skipRetryDelay {
			if err := waitForRetry(ctx, retryDelay); err != nil {
				releaseModel()
				return nil, err
			}
			retryDelay = nextRetryDelay(retryDelay, service.backendRetryMaxDelay)
		}
		skipRetryDelay = false

		response, err = service.forward(ctx, original, body)
		retryResult = service.backendRetryResult(response, err, original.URL.Path)
		if !retryResult.retry {
			return responseWithRelease(response, releaseModel), nil
		}

		lastStatus = retryResult.status
		lastErr = retryResult.err
		lastBody = retryResult.body
		service.logRetryableBackendResult(original.URL.Path, modelID, configFilename, attempt, lastStatus, lastErr, lastBody)

		if !recoveredBackend && service.shouldRecoverBackend(ctx, retryResult) {
			var recoveryErr error
			releaseModel, loadedFresh, recoveryErr = service.recoverBackendForModel(modelContext, releaseModel, modelID, configFilename, readiness, original.URL.Path, retryResult.err)
			if recoveryErr != nil {
				return nil, recoveryErr
			}
			recoveredBackend = true
		}
		if retryResult.inactive {
			if err := service.waitForInactiveBackend(modelContext, readiness, modelID, configFilename, original.URL.Path); err != nil {
				releaseModel()
				return nil, err
			}
			skipRetryDelay = true
		}
	}

	service.logger.Printf("backend retry exhausted path=%s model=%q config=%q attempts=%d status=%d error=%v body=%q", original.URL.Path, modelID, configFilename, service.backendRetryAttempts, lastStatus, lastErr, lastBody)
	releaseModel()
	return nil, backendRetryExhaustedError(lastStatus, lastErr, lastBody)
}

func (service *Service) shouldRecoverBackend(ctx context.Context, retryResult backendRetryResult) bool {
	if retryResult.err == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return !service.backend.Healthy(ctx)
}

func (service *Service) recoverBackendForModel(ctx context.Context, releaseModel func(), modelID string, configFilename string, readiness backendReadiness, path string, cause error) (func(), bool, error) {
	service.logger.Printf("backend transport recovery attempt path=%s model=%q config=%q error=%v", path, modelID, configFilename, cause)
	releaseModel()
	release, loadedFresh, err := service.acquireModelConfig(ctx, modelID, configFilename, readiness, true)
	if err != nil {
		return nil, false, err
	}
	service.logger.Printf("backend transport recovery succeeded path=%s model=%q config=%q reloaded=%t", path, modelID, configFilename, loadedFresh)
	return release, loadedFresh, nil
}

func (service *Service) waitForInactiveBackend(ctx context.Context, readiness backendReadiness, modelID string, configFilename string, path string) error {
	service.logger.Printf("backend inactive response path=%s model=%q config=%q", path, modelID, configFilename)
	return service.waitForBackendEndpoint(ctx, readiness, modelID, configFilename)
}

func (service *Service) backendRetryResult(response *http.Response, err error, path string) backendRetryResult {
	if err != nil {
		return backendRetryResult{retry: true, err: err}
	}
	if response == nil {
		return backendRetryResult{retry: true}
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

	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return backendRetryResult{retry: true, status: response.StatusCode, err: err}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return backendRetryResult{retry: true, status: response.StatusCode}
	}
	if isCorePath(path) && isJSONResponse(response.Header) {
		inactive := coreResponseIsInactive(body)
		if inactive || coreResponseHasEmptyText(body) {
			return backendRetryResult{retry: true, inactive: inactive, status: response.StatusCode, body: strings.TrimSpace(string(body))}
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
	_, _ = io.Copy(io.Discard, response.Body)
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

func (service *Service) acquireModelConfig(ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (func(), bool, error) {
	waitingSwitch := false
	for {
		service.activeConfigMu.Lock()
		if !force && service.activeConfigFilename == configFilename && !service.activeConfigSwitching && (service.activeConfigSwitchWaiters == 0 || waitingSwitch) {
			if waitingSwitch {
				service.activeConfigSwitchWaiters--
				service.notifyActiveConfigLocked()
			}
			service.activeConfigUsers++
			release := service.releaseActiveConfigOnce()
			service.activeConfigMu.Unlock()
			return release, false, nil
		}

		if !waitingSwitch && service.activeConfigSwitchWaiters > 0 {
			changed := service.activeConfigChanged
			service.activeConfigMu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return nil, false, err
			}
			continue
		}
		if !waitingSwitch {
			service.activeConfigSwitchWaiters++
			waitingSwitch = true
		}
		if service.activeConfigSwitching || service.activeConfigUsers > 0 {
			changed := service.activeConfigChanged
			service.activeConfigMu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				service.cancelConfigSwitchWaiter()
				return nil, false, err
			}
			continue
		}

		service.activeConfigSwitchWaiters--
		service.activeConfigSwitching = true
		service.activeConfigMu.Unlock()

		err := service.reloadModelConfig(ctx, modelID, configFilename)
		if err == nil {
			err = service.waitForBackendEndpoint(ctx, readiness, modelID, configFilename)
		}

		service.activeConfigMu.Lock()
		service.activeConfigSwitching = false
		if err != nil {
			service.activeConfigFilename = ""
			service.notifyActiveConfigLocked()
			service.activeConfigMu.Unlock()
			return nil, false, err
		}
		service.activeConfigFilename = configFilename
		service.activeConfigUsers++
		release := service.releaseActiveConfigOnce()
		service.notifyActiveConfigLocked()
		service.activeConfigMu.Unlock()
		return release, true, nil
	}
}

func (service *Service) cancelConfigSwitchWaiter() {
	service.activeConfigMu.Lock()
	if service.activeConfigSwitchWaiters > 0 {
		service.activeConfigSwitchWaiters--
		service.notifyActiveConfigLocked()
	}
	service.activeConfigMu.Unlock()
}

func (service *Service) releaseActiveConfigOnce() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			service.activeConfigMu.Lock()
			if service.activeConfigUsers > 0 {
				service.activeConfigUsers--
				if service.activeConfigUsers == 0 {
					service.notifyActiveConfigLocked()
				}
			}
			service.activeConfigMu.Unlock()
		})
	}
}

func (service *Service) notifyActiveConfigLocked() {
	close(service.activeConfigChanged)
	service.activeConfigChanged = make(chan struct{})
}

func waitForActiveConfigChange(ctx context.Context, changed <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	}
}

func (service *Service) currentConfigFilename() string {
	service.activeConfigMu.Lock()
	defer service.activeConfigMu.Unlock()
	return service.activeConfigFilename
}

func responseWithRelease(response *http.Response, release func()) *http.Response {
	if response == nil {
		release()
		return response
	}
	if response.Body == nil {
		release()
		return response
	}
	response.Body = &releaseReadCloser{
		ReadCloser: response.Body,
		release:    release,
	}
	return response
}

func (service *Service) reloadModelConfig(ctx context.Context, modelID string, configFilename string) error {
	service.logger.Printf("config switch reload attempt model=%q config=%q", modelID, configFilename)
	if reloadErr := service.backend.ReloadConfig(ctx, configFilename); reloadErr != nil {
		service.logger.Printf("config switch reload failed model=%q config=%q error=%v", modelID, configFilename, reloadErr)
		if service.backend.Healthy(ctx) {
			return reloadErr
		}

		service.logger.Printf("backend unhealthy after config switch failure model=%q config=%q", modelID, configFilename)
		restartContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		service.logger.Printf("kobold restart attempt model=%q config=%q", modelID, configFilename)
		if restartErr := service.backend.Restart(restartContext); restartErr != nil {
			service.logger.Printf("kobold restart failed model=%q config=%q error=%v", modelID, configFilename, restartErr)
			return fmt.Errorf("reload failed: %v; restart failed: %w", reloadErr, restartErr)
		}

		service.logger.Printf("config switch reload retry model=%q config=%q", modelID, configFilename)
		if retryErr := service.backend.ReloadConfig(ctx, configFilename); retryErr != nil {
			return fmt.Errorf("reload failed after restart: %w", retryErr)
		}
	}

	service.logger.Printf("config switch reload succeeded model=%q config=%q", modelID, configFilename)
	return nil
}

func (service *Service) waitForBackendEndpoint(ctx context.Context, readiness backendReadiness, modelID string, configFilename string) error {
	retryDelay := service.backendRetryDelay
	var lastStatus int
	var lastBody string
	var lastErr error

	for attempt := 1; attempt <= service.backendRetryAttempts; attempt++ {
		status, body, err := service.probeBackendEndpoint(ctx, string(readiness))
		if err == nil && backendEndpointReady(readiness, status, body) {
			if attempt > 1 {
				service.logger.Printf("backend model endpoint ready model=%q config=%q attempt=%d", modelID, configFilename, attempt)
			}
			return nil
		}

		lastStatus = status
		lastBody = body
		lastErr = err
		if attempt == 1 || attempt%30 == 0 {
			service.logger.Printf("waiting for backend model endpoint model=%q config=%q attempt=%d status=%d error=%v body=%q", modelID, configFilename, attempt, status, err, body)
		}

		if attempt < service.backendRetryAttempts {
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return err
			}
			retryDelay = nextRetryDelay(retryDelay, service.backendRetryMaxDelay)
		}
	}

	return fmt.Errorf("backend model endpoint unavailable after retries: status %d error=%v body=%q", lastStatus, lastErr, lastBody)
}

func backendEndpointReady(readiness backendReadiness, status int, body string) bool {
	if status < 200 || status >= 300 {
		return false
	}
	if readiness != readinessText {
		return true
	}
	return backendTextEndpointReady(body)
}

func backendTextEndpointReady(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	var parsed struct {
		Model string `json:"model"`
		ID    string `json:"id"`
		Data  []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(parsed.Model), "inactive") || strings.EqualFold(strings.TrimSpace(parsed.ID), "inactive") {
		return false
	}
	if parsed.Data == nil {
		return true
	}
	hasID := false
	for _, model := range parsed.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		hasID = true
		if !strings.EqualFold(id, "inactive") {
			return true
		}
	}
	return !hasID
}

func (service *Service) probeBackendEndpoint(ctx context.Context, path string) (int, string, error) {
	target := service.backend.URL()
	target.Path = joinPath(target.Path, path)
	target.RawQuery = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, "", err
	}

	response, err := service.client.Do(request)
	if err != nil {
		return 0, "", err
	}
	return response.StatusCode, drainResponseBodyPreview(response), nil
}

func (service *Service) forward(ctx context.Context, original *http.Request, body []byte) (*http.Response, error) {
	target := service.backend.URL()
	target.Path = joinPath(target.Path, original.URL.Path)
	target.RawQuery = original.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	copyRequestHeaders(request.Header, original.Header)
	request.Host = target.Host

	return service.client.Do(request)
}

func modelFromRequest(body []byte, r *http.Request) (string, bool, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") && !strings.HasSuffix(r.URL.Path, "/chat/completions") && !strings.HasSuffix(r.URL.Path, "/completions") {
		return "", false, nil
	}
	if len(body) == 0 {
		return "", false, nil
	}
	return openai.ModelFromJSON(body)
}

func imageModelFromRequest(body []byte, r *http.Request) (string, bool, error) {
	if len(body) > 0 && requestBodyLooksJSON(body, r) {
		modelID, ok, err := imageModelFromJSON(body)
		if err != nil || ok {
			return modelID, ok, err
		}
	}
	if modelID := strings.TrimSpace(r.URL.Query().Get("model")); modelID != "" {
		return modelID, true, nil
	}
	if modelID := strings.TrimSpace(r.URL.Query().Get("sd_model_checkpoint")); modelID != "" {
		return modelID, true, nil
	}
	if modelID := strings.TrimSpace(r.Header.Get("X-Tensors-Model")); modelID != "" {
		return modelID, true, nil
	}
	return "", false, nil
}

func requestBodyLooksJSON(body []byte, r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func imageModelFromJSON(body []byte) (string, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", false, err
	}
	if modelID, ok, err := jsonStringSelector(raw, "model"); err != nil || ok {
		return modelID, ok, err
	}
	if modelID, ok, err := jsonStringSelector(raw, "sd_model_checkpoint"); err != nil || ok {
		return modelID, ok, err
	}
	overrideSettings, ok := raw["override_settings"]
	if !ok {
		return "", false, nil
	}
	var overrideRaw map[string]json.RawMessage
	if err := json.Unmarshal(overrideSettings, &overrideRaw); err != nil {
		return "", false, fmt.Errorf("override_settings must be an object")
	}
	return jsonStringSelector(overrideRaw, "sd_model_checkpoint")
}

func jsonStringSelector(raw map[string]json.RawMessage, key string) (string, bool, error) {
	value, ok := raw[key]
	if !ok {
		return "", false, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", true, fmt.Errorf("%s must be a string", key)
	}
	text = strings.TrimSpace(text)
	return text, text != "", nil
}

func writeProxyResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error {
	defer response.Body.Close()

	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isEventStream(response.Header) {
		return writeEventStreamResponse(w, response, virtualModelID)
	}
	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isJSONResponse(response.Header) {
		return writeJSONResponseWithVirtualModel(w, response, virtualModelID)
	}

	copyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)

	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 32*1024)
	for {
		read, readErr := response.Body.Read(buffer)
		if read > 0 {
			if _, err := w.Write(buffer[:read]); err != nil {
				return err
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func writeJSONResponseWithVirtualModel(w http.ResponseWriter, response *http.Response, virtualModelID string) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	body = rewriteJSONModel(body, virtualModelID)
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	_, err = w.Write(body)
	return err
}

func writeEventStreamResponse(w http.ResponseWriter, response *http.Response, virtualModelID string) error {
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)

	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			line = "data: " + rewriteEventDataModel(strings.TrimPrefix(line, "data: "), virtualModelID)
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
		if line == "" && flusher != nil {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func rewriteJSONModel(body []byte, virtualModelID string) []byte {
	rewritten, ok := rewriteTopLevelStringField(body, "model", virtualModelID)
	if !ok {
		return body
	}
	return rewritten
}

func rewriteEventDataModel(data string, virtualModelID string) string {
	if data == "[DONE]" {
		return data
	}
	rewritten := rewriteJSONModel([]byte(data), virtualModelID)
	return string(rewritten)
}

func rewriteTopLevelStringField(body []byte, fieldName string, fieldValue string) ([]byte, bool) {
	quotedFieldName := []byte(strconv.Quote(fieldName))
	quotedFieldValue := []byte(strconv.Quote(fieldValue))
	depth := 0
	inString := false
	escaped := false

	for index := 0; index < len(body); index++ {
		char := body[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}

		switch char {
		case '"':
			if depth != 1 {
				inString = true
				continue
			}
			keyEnd := findJSONStringEnd(body, index)
			if keyEnd == -1 {
				return body, false
			}
			if !bytes.Equal(body[index:keyEnd], quotedFieldName) {
				index = keyEnd - 1
				continue
			}
			colonIndex := skipWhitespace(body, keyEnd)
			if colonIndex >= len(body) || body[colonIndex] != ':' {
				return body, false
			}
			valueStart := skipWhitespace(body, colonIndex+1)
			if valueStart >= len(body) || body[valueStart] != '"' {
				return body, false
			}
			valueEnd := findJSONStringEnd(body, valueStart)
			if valueEnd == -1 {
				return body, false
			}
			rewritten := make([]byte, 0, len(body)+len(quotedFieldValue)-(valueEnd-valueStart))
			rewritten = append(rewritten, body[:valueStart]...)
			rewritten = append(rewritten, quotedFieldValue...)
			rewritten = append(rewritten, body[valueEnd:]...)
			return rewritten, true
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}

	return body, false
}

func findJSONStringEnd(body []byte, start int) int {
	escaped := false
	for index := start + 1; index < len(body); index++ {
		char := body[index]
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			return index + 1
		}
	}
	return -1
}

func skipWhitespace(body []byte, start int) int {
	for start < len(body) {
		switch body[start] {
		case ' ', '\n', '\r', '\t':
			start++
		default:
			return start
		}
	}
	return start
}

func copyRequestHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func isJSONResponse(header http.Header) bool {
	return strings.Contains(strings.ToLower(header.Get("Content-Type")), "application/json")
}

func isEventStream(header http.Header) bool {
	return strings.Contains(strings.ToLower(header.Get("Content-Type")), "text/event-stream")
}

func isImagePath(path string) bool {
	if strings.HasPrefix(path, "/v1/images/") {
		return true
	}
	if strings.HasPrefix(path, "/sdapi/v1/") {
		return true
	}
	switch path {
	case "/prompt", "/queue", "/history", "/view", "/object_info", "/system_stats", "/interrupt":
		return true
	default:
		return strings.HasPrefix(path, "/history/") ||
			strings.HasPrefix(path, "/view/") ||
			strings.HasPrefix(path, "/object_info/") ||
			strings.HasPrefix(path, "/upload/image")
	}
}

func isCorePath(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/completions"
}

func isOpenAIPath(path string) bool {
	return path == "/v1" || strings.HasPrefix(path, "/v1/")
}

func joinPath(base string, requestPath string) string {
	if base == "" || base == "/" {
		return requestPath
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(requestPath, "/")
}
