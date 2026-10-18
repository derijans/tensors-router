package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestOllamaChatReachesLlamaAsAChatCompletion(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"gemma": `{"backend_mode":"llama_sdcpp","model_param":"model.gguf"}`,
	})
	var mutex sync.Mutex
	var backendPaths []string
	service.client = chatTemplateTestClient(t, func(request *http.Request) {
		mutex.Lock()
		backendPaths = append(backendPaths, request.URL.Path)
		mutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, pathOllamaChat, strings.NewReader(`{"model":"gemma","stream":false,"messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", recorder.Code, recorder.Body.String())
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(backendPaths) != 1 || backendPaths[0] != "/v1/chat/completions" {
		t.Fatalf("backend paths = %v, want the OpenAI chat endpoint", backendPaths)
	}
	var record map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &record); err != nil {
		t.Fatalf("body %s is not JSON: %v", recorder.Body.String(), err)
	}
	if record["model"] != "gemma" || record["done"] != true || record["message"].(map[string]any)["content"] != "ready" {
		t.Fatalf("record = %v, want an Ollama chat answer", record)
	}
}

func TestOllamaChatCarriesTheConfigsJinjaKwargs(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"instruct": `{"backend_mode":"llama_sdcpp","model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false}}`,
	})
	captured := &capturedBackendRequests{}
	service.client = chatTemplateTestClient(t, captured.record(t))

	postToPath(t, service, pathOllamaChat, `{"model":"instruct","stream":false,"think":true,"messages":[{"role":"user","content":"hi"}]}`)

	requests := captured.all()
	kwargs, ok := requests[0]["chat_template_kwargs"].(map[string]any)
	if !ok || kwargs["enable_thinking"] != false {
		t.Fatalf("backend request = %v, want the instruct config's enable_thinking=false", requests[0])
	}
}

func TestInvalidOllamaChatToLlamaIsAClientError(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"gemma": `{"backend_mode":"llama_sdcpp","model_param":"model.gguf"}`,
	})
	service.client = chatTemplateTestClient(t, func(*http.Request) {
		t.Error("an invalid Ollama request reached the backend")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, pathOllamaChat, strings.NewReader(`{"model":"gemma","messages":"hi"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"error"`) {
		t.Fatalf("status %d body=%s, want an Ollama 400", recorder.Code, recorder.Body.String())
	}
}
