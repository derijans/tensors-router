package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/ollama"
)

type ollamaModel struct {
	Name         string         `json:"name"`
	Model        string         `json:"model"`
	ModifiedAt   time.Time      `json:"modified_at"`
	Size         int64          `json:"size"`
	Digest       string         `json:"digest"`
	Details      map[string]any `json:"details"`
	Capabilities []string       `json:"capabilities,omitempty"`
	SizeVRAM     int64          `json:"size_vram,omitempty"`
}

// modelRuntimeLoaded reports whether the runtime serving this model is up. An
// embedding-only model is resident on the embeddings runtime, not the text one.
func modelRuntimeLoaded(model cluster.Model) bool {
	if model.HasLLM {
		return model.Loaded
	}
	return model.EmbeddingsLoaded || model.Loaded
}

func isOllamaPath(path string) bool {
	_, ok := ollama.Method(path)
	return ok
}

func rejectOllamaMethod(w http.ResponseWriter, r *http.Request) bool {
	allowedMethod, ok := ollama.Method(r.URL.Path)
	if !ok || r.Method == allowedMethod {
		return false
	}
	w.Header().Set("Allow", allowedMethod)
	ollama.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	return true
}

func (service *Service) handleOllamaRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/tags":
		service.handleOllamaTags(w, r)
	case "/api/ps":
		service.handleOllamaPS(w, r)
	case "/api/version":
		ollamaVersion := buildinfo.Current().Version
		writeOllamaJSON(w, http.StatusOK, map[string]string{"version": ollamaVersion})
	case "/api/show":
		service.handleOllamaShow(w, r)
	case "/api/embed":
		service.handleModelRequest(w, r, false)
	default:
		service.handleModelRequest(w, r, true)
	}
}

type ollamaShowRequest struct {
	Model string `json:"model"`
	Name  string `json:"name"`
}

type ollamaShowResponse struct {
	Details      ollamaShowDetails `json:"details,omitempty"`
	ModelInfo    map[string]any    `json:"model_info"`
	Capabilities []string          `json:"capabilities,omitempty"`
	ModifiedAt   time.Time         `json:"modified_at,omitempty"`
}

type ollamaShowDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
	ContextLength     int64    `json:"context_length,omitempty"`
}

// handleOllamaShow answers model metadata from the catalog. Ollama's own ShowHandler
// reads model metadata off disk and never schedules the model, so neither do we: this
// must not acquire, load, or switch a runtime.
func (service *Service) handleOllamaShow(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, analyticsRequestMetadataLimit))
	if err != nil {
		ollama.WriteError(w, http.StatusBadRequest, "request body could not be read")
		return
	}
	defer r.Body.Close()

	var parsed ollamaShowRequest
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &parsed); err != nil {
			ollama.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	name := strings.TrimSpace(parsed.Model)
	if name == "" {
		name = strings.TrimSpace(parsed.Name)
	}
	if name == "" {
		ollama.WriteError(w, http.StatusBadRequest, "model is required")
		return
	}

	models, err := service.ollamaVisibleModels(r.Context())
	if err != nil {
		ollama.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, model := range models {
		if model.PublicID != name {
			continue
		}
		writeOllamaJSON(w, http.StatusOK, ollamaShowPayload(model))
		return
	}
	ollama.WriteError(w, http.StatusNotFound, fmt.Sprintf("model '%s' not found", name))
}

func ollamaShowPayload(model cluster.Model) ollamaShowResponse {
	modifiedAt := time.Unix(model.Created, 0).UTC()
	if model.Created <= 0 {
		modifiedAt = time.Unix(0, 0).UTC()
	}
	details := ollamaShowDetails{
		Format:            "gguf",
		Family:            ollamaFamily(model),
		Families:          []string{ollamaFamily(model)},
		ParameterSize:     "unknown",
		QuantizationLevel: "unknown",
		ContextLength:     ollamaContextLength(model),
	}
	// model_info is the one field Ollama does not mark omitempty, so clients may rely on
	// the key existing even when we have no GGUF metadata to put in it.
	info := map[string]any{}
	if details.ContextLength > 0 {
		info[details.Family+".context_length"] = details.ContextLength
	}
	return ollamaShowResponse{
		Details:      details,
		ModelInfo:    info,
		Capabilities: ollamaCapabilities(model),
		ModifiedAt:   modifiedAt,
	}
}

// ollamaCapabilities reports the capability vocabulary Ollama uses. Upstream derives
// "embedding" from a pooling_type key and treats it as mutually exclusive with
// "completion", so an embedding-only model must not also claim completion.
func ollamaCapabilities(model cluster.Model) []string {
	if model.HasEmbeddings && !model.HasLLM {
		return []string{"embedding"}
	}
	capabilities := make([]string, 0, 4)
	if model.HasLLM {
		capabilities = append(capabilities, "completion")
	}
	if model.HasMultimodal {
		capabilities = append(capabilities, "vision")
	}
	if model.MCPEnabled {
		capabilities = append(capabilities, "tools")
	}
	if model.HasVoice {
		capabilities = append(capabilities, "audio")
	}
	if model.HasImage {
		capabilities = append(capabilities, "image")
	}
	if model.HasEmbeddings {
		capabilities = append(capabilities, "embedding")
	}
	return capabilities
}

// ollamaFamily reports the model architecture. The catalog carries no GGUF metadata, so
// this is "unknown" rather than the backend name — the backend is not an architecture.
func ollamaFamily(model cluster.Model) string {
	return "unknown"
}

func ollamaContextLength(model cluster.Model) int64 {
	raw, ok := model.Options["contextsize"]
	if !ok {
		return 0
	}
	var size int64
	if err := json.Unmarshal(raw, &size); err != nil {
		return 0
	}
	if size < 0 {
		return 0
	}
	return size
}

func (service *Service) handleOllamaTags(w http.ResponseWriter, r *http.Request) {
	models, err := service.ollamaVisibleModels(r.Context())
	if err != nil {
		ollama.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOllamaJSON(w, http.StatusOK, map[string]any{"models": ollamaModels(models, false)})
}

func (service *Service) handleOllamaPS(w http.ResponseWriter, r *http.Request) {
	models, err := service.ollamaVisibleModels(r.Context())
	if err != nil {
		ollama.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOllamaJSON(w, http.StatusOK, map[string]any{"models": ollamaModels(models, true)})
}

func (service *Service) ollamaVisibleModels(ctx context.Context) ([]cluster.Model, error) {
	if service.registry != nil {
		return service.modelsWithRuntimeState(ctx, service.registry.Models()), nil
	}
	// List, not ListLLM: an embedding-only model is servable over /api/embed and must be
	// discoverable, but ListLLM filters on HasLLM and would hide it.
	all, err := service.catalog.List()
	if err != nil {
		return nil, err
	}
	models := make([]catalog.Model, 0, len(all))
	for _, model := range all {
		if model.HasLLM || model.HasEmbeddings {
			models = append(models, model)
		}
	}
	localModels := cluster.LocalModelsWithBackendMode(models, service.nodeID, service.nodeURL, service.localSource(), service.backendMode)
	return service.modelsWithRuntimeState(ctx, localModels), nil
}

func (service *Service) modelsWithRuntimeState(ctx context.Context, models []cluster.Model) []cluster.Model {
	loadedFiles := map[string]bool{}
	loadedEmbeddingFiles := map[string]bool{}
	for _, family := range service.backendFamilies {
		if family == nil {
			continue
		}
		if family.textRuntime != nil && family.textRuntime.backend != nil && family.textRuntime.backend.Healthy(ctx) {
			filename := currentRuntimeConfigFilename(family.textRuntime)
			if filename != "" {
				loadedFiles[filename] = true
			}
		}
		if family.embeddingsRuntime != nil && family.embeddingsRuntime.backend != nil && family.embeddingsRuntime.backend.Healthy(ctx) {
			filename := currentRuntimeConfigFilename(family.embeddingsRuntime)
			if filename != "" {
				loadedEmbeddingFiles[filename] = true
			}
		}
	}
	result := make([]cluster.Model, len(models))
	copy(result, models)
	for index := range result {
		if result[index].NodeID == service.nodeID && loadedFiles[result[index].Filename] {
			result[index].Loaded = true
		}
		if result[index].NodeID == service.nodeID && result[index].HasEmbeddings && loadedEmbeddingFiles[result[index].Filename] {
			result[index].EmbeddingsLoaded = true
		}
	}
	return result
}

func ollamaModels(models []cluster.Model, loadedOnly bool) []ollamaModel {
	seen := map[string]struct{}{}
	result := make([]ollamaModel, 0, len(models))
	for _, model := range models {
		// Ollama lists embedding models alongside chat models, and /api/embed already
		// serves them here, so hiding them from discovery leaves clients unable to name a
		// model they can actually use.
		servable := model.HasLLM || model.HasEmbeddings
		if !servable || strings.TrimSpace(model.PublicID) == "" || loadedOnly && (!model.Available || !modelRuntimeLoaded(model)) {
			continue
		}
		if _, exists := seen[model.PublicID]; exists {
			continue
		}
		seen[model.PublicID] = struct{}{}
		modifiedAt := time.Unix(model.Created, 0).UTC()
		if model.Created <= 0 {
			modifiedAt = time.Unix(0, 0).UTC()
		}
		record := ollamaModel{
			Name:         model.PublicID,
			Model:        model.PublicID,
			ModifiedAt:   modifiedAt,
			Size:         model.Size,
			Digest:       ollamaDigest(model),
			Details:      ollamaDetails(model),
			Capabilities: ollamaCapabilities(model),
		}
		if loadedOnly {
			record.SizeVRAM = model.Size
		}
		result = append(result, record)
	}
	sort.Slice(result, func(left int, right int) bool { return result[left].Name < result[right].Name })
	return result
}

func ollamaDigest(model cluster.Model) string {
	value := strings.TrimSpace(model.ModelHash)
	if value == "" {
		value = strings.TrimSpace(model.ConfigHash)
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == sha256.Size {
		return "sha256:" + strings.ToLower(value)
	}
	digest := sha256.Sum256([]byte(model.PublicID + "\x00" + value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

// ollamaDetails mirrors Ollama's ModelDetails, where family/families name the model
// architecture. Capabilities belong in the separate capabilities array, and the backend
// that happens to serve the model is not an architecture.
func ollamaDetails(model cluster.Model) map[string]any {
	family := ollamaFamily(model)
	return map[string]any{
		"format":             "gguf",
		"family":             family,
		"families":           []string{family},
		"parameter_size":     "unknown",
		"quantization_level": "unknown",
	}
}

const maxNDJSONRecordBytes = 4 * 1024 * 1024

func writeNDJSONResponseWithVirtualModel(w http.ResponseWriter, response *http.Response, virtualModelID string) error {
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(response.Body)
	for {
		record, readErr := readBoundedNDJSONRecord(reader)
		hasNewline := len(record) > 0 && record[len(record)-1] == '\n'
		line := record
		if hasNewline {
			line = line[:len(line)-1]
		}
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) > 0 {
			if !json.Valid(line) {
				return fmt.Errorf("backend returned invalid ndjson")
			}
			line = htmlEscapeJSON(rewriteJSONModel(line, virtualModelID))
			if _, writeErr := w.Write(line); writeErr != nil {
				return writeErr
			}
		}
		if hasNewline {
			if _, writeErr := io.WriteString(w, "\n"); writeErr != nil {
				return writeErr
			}
		}
		if flusher != nil && len(record) > 0 {
			flusher.Flush()
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func readBoundedNDJSONRecord(reader *bufio.Reader) ([]byte, error) {
	record := make([]byte, 0, 4096)
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(record)+len(fragment) > maxNDJSONRecordBytes {
			return nil, fmt.Errorf("backend ndjson record exceeds %d bytes", maxNDJSONRecordBytes)
		}
		record = append(record, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			return record, io.EOF
		}
		return record, err
	}
}
func isNDJSONResponse(header http.Header) bool {
	contentType := strings.ToLower(header.Get("Content-Type"))
	return strings.Contains(contentType, "application/x-ndjson") || strings.Contains(contentType, "application/ndjson")
}
func writeOllamaJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
