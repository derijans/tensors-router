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
	"tensors-router/internal/cluster"
	"tensors-router/internal/ollama"
)

type ollamaModel struct {
	Name       string         `json:"name"`
	Model      string         `json:"model"`
	ModifiedAt time.Time      `json:"modified_at"`
	Size       int64          `json:"size"`
	Digest     string         `json:"digest"`
	Details    map[string]any `json:"details"`
	SizeVRAM   int64          `json:"size_vram,omitempty"`
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
	default:
		service.handleModelRequest(w, r, true)
	}
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
	models, err := service.catalog.ListLLM()
	if err != nil {
		return nil, err
	}
	localModels := cluster.LocalModelsWithBackendMode(models, service.nodeID, service.nodeURL, service.localSource(), service.backendMode)
	return service.modelsWithRuntimeState(ctx, localModels), nil
}

func (service *Service) modelsWithRuntimeState(ctx context.Context, models []cluster.Model) []cluster.Model {
	loadedFiles := map[string]bool{}
	for _, family := range service.backendFamilies {
		if family == nil || family.textRuntime == nil || family.textRuntime.backend == nil || !family.textRuntime.backend.Healthy(ctx) {
			continue
		}
		filename := currentRuntimeConfigFilename(family.textRuntime)
		if filename != "" {
			loadedFiles[filename] = true
		}
	}
	result := make([]cluster.Model, len(models))
	copy(result, models)
	for index := range result {
		if result[index].NodeID == service.nodeID && loadedFiles[result[index].Filename] {
			result[index].Loaded = true
		}
	}
	return result
}

func ollamaModels(models []cluster.Model, loadedOnly bool) []ollamaModel {
	seen := map[string]struct{}{}
	result := make([]ollamaModel, 0, len(models))
	for _, model := range models {
		if !model.HasLLM || strings.TrimSpace(model.PublicID) == "" || loadedOnly && (!model.Available || !model.Loaded) {
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
			Name:       model.PublicID,
			Model:      model.PublicID,
			ModifiedAt: modifiedAt,
			Size:       model.Size,
			Digest:     ollamaDigest(model),
			Details:    ollamaDetails(model),
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

func ollamaDetails(model cluster.Model) map[string]any {
	families := []string{}
	if model.HasLLM {
		families = append(families, "text")
	}
	if model.HasMultimodal {
		families = append(families, "multimodal")
	}
	if model.HasEmbeddings {
		families = append(families, "embedding")
	}
	family := strings.TrimSpace(model.BackendMode)
	if family == "" {
		family = "unknown"
	}
	return map[string]any{
		"format":             "gguf",
		"family":             family,
		"families":           families,
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
