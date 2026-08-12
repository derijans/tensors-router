package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func TestVLLMInferenceAllowlist(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/pooling"},
		{http.MethodPost, "/v2/embed"},
		{http.MethodPost, "/generative_scoring"},
		{http.MethodGet, "/v1/responses/resp-1"},
		{http.MethodPost, "/v1/responses/resp-1/cancel"},
		{http.MethodDelete, "/v1/responses/resp-1"},
	}
	for _, route := range allowed {
		if !vllmInferenceAllowed(route.method, route.path) {
			t.Fatalf("expected %s %s to be allowed", route.method, route.path)
		}
	}
	blocked := []struct{ method, path string }{
		{http.MethodGet, "/v1/chat/completions"},
		{http.MethodPost, "/rpc"},
		{http.MethodPost, "/v1/responses/resp-1/profiler"},
		{http.MethodPost, "/v1/load_lora_adapter"},
		{http.MethodPost, "/sleep"},
		{http.MethodPost, "/v1/responses/../cancel"},
	}
	for _, route := range blocked {
		if vllmInferenceAllowed(route.method, route.path) {
			t.Fatalf("expected %s %s to be blocked", route.method, route.path)
		}
	}
}

func TestVLLMRequestModelIDPreservesServedNames(t *testing.T) {
	servedNames := []string{"public-model", "adapter-one"}
	if got := vllmRequestModelID("adapter-one", "model", servedNames); got != "adapter-one" {
		t.Fatalf("static adapter identity was rewritten: %q", got)
	}
	if got := vllmRequestModelID("recipe-alias", "model", servedNames); got != "model" {
		t.Fatalf("router-only alias reached runtime: %q", got)
	}
}

func TestSelectorlessVLLMSelectionPrefersLoadedAndRejectsAmbiguity(t *testing.T) {
	models := []cluster.Model{
		{PublicID: "idle", LocalID: "idle", Filename: "idle.kcpps", ConfigHash: "idle-hash", BackendMode: BackendModeVLLM, HasLLM: true, Available: true, VLLMTask: "generate"},
		{PublicID: "loaded", LocalID: "loaded", Filename: "loaded.kcpps", ConfigHash: "loaded-hash", BackendMode: BackendModeVLLM, HasLLM: true, Available: true, Loaded: true, VLLMTask: "generate"},
	}
	selected, err := selectClusterVLLMModel(models, "/generative_scoring")
	if err != nil || selected != "loaded" {
		t.Fatalf("loaded model was not preferred selected=%q error=%v", selected, err)
	}
	models[0].Loaded = true
	if _, err := selectClusterVLLMModel(models, "/generative_scoring"); err == nil || !strings.Contains(err.Error(), "multiple compatible") {
		t.Fatalf("ambiguous loaded models were accepted: %v", err)
	}
	models[0].Loaded = false
	models[1].Loaded = false
	if _, err := selectClusterVLLMModel(models, "/generative_scoring"); err == nil || !strings.Contains(err.Error(), "multiple compatible") {
		t.Fatalf("ambiguous idle models were accepted: %v", err)
	}
}

func TestVLLMTaskPathCompatibilityIsExact(t *testing.T) {
	tests := []struct {
		task string
		path string
		want bool
	}{
		{"generate", "/v1/chat/completions", true},
		{"generate", "/v1/embeddings", false},
		{"embedding", "/v1/embeddings", true},
		{"embedding", "/v1/chat/completions", false},
		{"classification", "/classify", true},
		{"classification", "/v1/embeddings", false},
		{"score", "/v2/rerank", true},
		{"score", "/classify", false},
		{"pooling", "/pooling", true},
		{"pooling", "/v1/embeddings", false},
		{"pooling", "/tokenize", true},
		{"transcription", "/tokenize", false},
	}
	for _, test := range tests {
		catalogModel := catalog.Model{BackendMode: BackendModeVLLM, VLLMTask: test.task, HasLLM: true, HasEmbeddings: true}
		clusterModel := cluster.Model{BackendMode: BackendModeVLLM, VLLMTask: test.task, HasLLM: true, HasEmbeddings: true}
		if got := modelSupportsOpenAIPath(catalogModel, test.path); got != test.want {
			t.Errorf("catalog task=%s path=%s got=%t want=%t", test.task, test.path, got, test.want)
		}
		if got := registryModelSupportsOpenAIPath(clusterModel, test.path); got != test.want {
			t.Errorf("cluster task=%s path=%s got=%t want=%t", test.task, test.path, got, test.want)
		}
	}
}

func TestVLLMModelsIncludeRuntimeTasksAndAliases(t *testing.T) {
	configDir := t.TempDir()
	configs := map[string]string{
		"generate": `{"backend_mode":"vllm","vllm":{"task":"generate","served_names":["generate-alias"]}}`,
		"embed":    `{"backend_mode":"vllm","vllm":{"task":"embedding","served_names":["embed-alias"]}}`,
		"speech":   `{"backend_mode":"vllm","vllm":{"task":"transcription","served_names":["speech-alias"]}}`,
		"image":    `{"backend_mode":"llama-sd.cpp","nomodel":true,"sdmodel":"image.safetensors"}`,
	}
	for name, content := range configs {
		if err := os.WriteFile(filepath.Join(configDir, name+".kcpps"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(ServiceConfig{Catalog: catalog.New(configDir), Logger: log.New(io.Discard, "", 0)})
	defer service.Close(t.Context())
	assertVLLMModelIDs(t, service, []string{"generate", "generate-alias", "embed", "embed-alias", "speech", "speech-alias"}, []string{"image"})
}

func TestClusterVLLMModelsIncludeRuntimeTasksAndAliases(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleMaster, "local", "http://local")
	models := []cluster.Model{
		{LocalID: "generate", Filename: "generate.kcpps", BackendMode: BackendModeVLLM, HasLLM: true, VLLMTask: "generate", ServedNames: []string{"generate-alias"}},
		{LocalID: "embed", Filename: "embed.kcpps", BackendMode: BackendModeVLLM, HasEmbeddings: true, VLLMTask: "embedding", ServedNames: []string{"embed-alias"}},
		{LocalID: "speech", Filename: "speech.kcpps", BackendMode: BackendModeVLLM, HasVoice: true, VLLMTask: "transcription", ServedNames: []string{"speech-alias"}},
		{LocalID: "other-embed", Filename: "other.kcpps", BackendMode: BackendModeKobold, HasEmbeddings: true},
	}
	if err := registry.UpdateLocal(models); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Registry: registry, Logger: log.New(io.Discard, "", 0)})
	defer service.Close(t.Context())
	assertVLLMModelIDs(t, service, []string{"generate", "generate-alias", "embed", "embed-alias", "speech", "speech-alias"}, []string{"other-embed"})
}

func assertVLLMModelIDs(t *testing.T, service *Service, included []string, excluded []string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected models status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response openai.ModelsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool, len(response.Data))
	for _, model := range response.Data {
		ids[model.ID] = true
	}
	for _, id := range included {
		if !ids[id] {
			t.Errorf("model %q missing from %#v", id, ids)
		}
	}
	for _, id := range excluded {
		if ids[id] {
			t.Errorf("model %q unexpectedly exposed in %#v", id, ids)
		}
	}
}

func TestSelectorlessVLLMRequestRejectsAmbiguousCatalog(t *testing.T) {
	configDir := t.TempDir()
	for _, name := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(configDir, name+".kcpps"), []byte(`{"backend_mode":"vllm","vllm":{"task":"generative_scoring"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backendURL, err := url.Parse("http://unused.invalid")
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {TextBackend: backend, EmbeddingsBackend: backend, TranscriptionBackend: backend},
		},
		Catalog: catalog.New(configDir),
		Logger:  log.New(io.Discard, "", 0),
	})
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/generative_scoring", strings.NewReader(`{"prompt":"hello"}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"type":"ambiguous_model_selector"`) {
		t.Fatalf("unexpected ambiguity response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUninitializedVLLMInferenceReturnsServiceUnavailable(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("uninitialized runtime received inference request")
	}))
	defer backendServer.Close()
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		url:       backendURL,
		reloadErr: func(string) error { return errors.New("backend_not_initialized") },
	}
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "model.kcpps"), []byte(`{"backend_mode":"vllm","vllm":{"task":"generate"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {TextBackend: backend, EmbeddingsBackend: backend, TranscriptionBackend: backend},
		},
		Catalog:   catalog.New(configDir),
		ConfigDir: configDir,
		Logger:    log.New(io.Discard, "", 0),
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"type":"backend_not_initialized"`) {
		t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() == 0 || backend.restarts.Load() == 0 {
		t.Fatalf("model lifecycle was not attempted reloads=%d restarts=%d", backend.reloads.Load(), backend.restarts.Load())
	}
}
