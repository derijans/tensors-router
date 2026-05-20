package proxy

import (
	"bufio"
	"bytes"
	"context"
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
	Resolve(id string) (catalog.Model, bool, error)
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

	activeConfigMu       sync.Mutex
	activeConfigFilename string
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
	}
}

func (service *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		service.handleModels(w)
		return
	}

	if r.Method == http.MethodPost && isCorePath(r.URL.Path) {
		service.handleModelRequest(w, r, true)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/v1/") {
		service.handleModelRequest(w, r, false)
		return
	}

	openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
}

func (service *Service) handleModels(w http.ResponseWriter) {
	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, openai.ModelsResponseFromCatalog(models))
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
		configFilename = model.Filename
	}

	response, err := service.forwardWithFallback(r.Context(), r, body, modelID, configFilename, hasModel)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}

	if err := writeProxyResponse(w, response, modelID, hasModel); err != nil {
		return
	}
}

func (service *Service) forwardWithFallback(ctx context.Context, original *http.Request, body []byte, modelID string, configFilename string, hasModel bool) (*http.Response, error) {
	loadedFresh := false
	if hasModel {
		var err error
		loadedFresh, err = service.ensureModelConfig(ctx, modelID, configFilename)
		if err != nil {
			return nil, err
		}
	}

	response, err := service.forward(ctx, original, body)
	if !hasModel {
		return response, err
	}
	if err == nil && response.StatusCode < 500 {
		return response, nil
	}
	if err != nil {
		service.logger.Printf("backend request failed path=%s model=%q config=%q error=%v", original.URL.Path, modelID, configFilename, err)
	} else {
		service.logger.Printf("backend returned retryable status path=%s model=%q config=%q status=%d", original.URL.Path, modelID, configFilename, response.StatusCode)
	}
	if response != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}

	if !loadedFresh {
		if reloadErr := service.forceModelConfig(ctx, modelID, configFilename); reloadErr != nil {
			return nil, reloadErr
		}
	} else {
		service.logger.Printf("backend retry after fresh config load model=%q config=%q", modelID, configFilename)
	}

	response, err = service.forward(ctx, original, body)
	if err != nil {
		service.logger.Printf("backend retry failed path=%s model=%q config=%q error=%v", original.URL.Path, modelID, configFilename, err)
	}
	return response, err
}

func (service *Service) ensureModelConfig(ctx context.Context, modelID string, configFilename string) (bool, error) {
	service.activeConfigMu.Lock()
	defer service.activeConfigMu.Unlock()

	if service.activeConfigFilename == configFilename {
		return false, nil
	}

	if err := service.reloadModelConfigLocked(ctx, modelID, configFilename); err != nil {
		return false, err
	}
	return true, nil
}

func (service *Service) forceModelConfig(ctx context.Context, modelID string, configFilename string) error {
	service.activeConfigMu.Lock()
	defer service.activeConfigMu.Unlock()

	return service.reloadModelConfigLocked(ctx, modelID, configFilename)
}

func (service *Service) reloadModelConfigLocked(ctx context.Context, modelID string, configFilename string) error {
	service.logger.Printf("config switch reload attempt model=%q config=%q", modelID, configFilename)
	if reloadErr := service.backend.ReloadConfig(ctx, configFilename); reloadErr != nil {
		service.activeConfigFilename = ""
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

	service.activeConfigFilename = configFilename
	service.logger.Printf("config switch reload succeeded model=%q config=%q", modelID, configFilename)
	return nil
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

func isCorePath(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/completions"
}

func joinPath(base string, requestPath string) string {
	if base == "" || base == "/" {
		return requestPath
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(requestPath, "/")
}
