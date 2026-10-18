package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tensors-router/internal/catalog"
)

var samplerVariantConfigs = map[string]string{
	"instruct": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false},"gendefaults":"{\"temperature\": 0.7, \"top_p\": 0.8}"}`,
	"think":    `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true},"gendefaults":"{\"temperature\": 1.0, \"top_p\": 0.95}"}`,
}

type capturedBackendRequests struct {
	mu       sync.Mutex
	requests []map[string]any
}

func (captured *capturedBackendRequests) record(t *testing.T) func(*http.Request) {
	return func(request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var value map[string]any
		if err := json.Unmarshal(content, &value); err != nil {
			t.Errorf("backend body %s is not JSON: %v", content, err)
			return
		}
		captured.mu.Lock()
		captured.requests = append(captured.requests, value)
		captured.mu.Unlock()
	}
}

func (captured *capturedBackendRequests) all() []map[string]any {
	captured.mu.Lock()
	defer captured.mu.Unlock()
	return append([]map[string]any(nil), captured.requests...)
}

func serviceWithSamplerVariants(t *testing.T, policy catalog.RequestProfilePolicy) (*Service, *fakeBackend, *capturedBackendRequests) {
	t.Helper()
	service, backend := newTestServiceWithRequestProfiles(t, http.NotFoundHandler(), samplerVariantConfigs, policy)
	captured := &capturedBackendRequests{}
	service.client = chatTemplateTestClient(t, captured.record(t))
	return service, backend, captured
}

func postToPath(t *testing.T, service *Service, path string, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s status %d body=%s", path, recorder.Code, recorder.Body.String())
	}
}

func TestSamplerVariantsShareOneRuntimeAndCarryTheirOwnDefaults(t *testing.T) {
	service, backend, captured := serviceWithSamplerVariants(t, catalog.RequestProfilePolicy{InjectGenerationDefaults: true, ShareRuntimeByLaunchArguments: true, DefaultBackendMode: "kobold"})

	postChatTemplateRequest(t, service, `{"model":"instruct","messages":[]}`)
	postChatTemplateRequest(t, service, `{"model":"think","messages":[]}`)
	postChatTemplateRequest(t, service, `{"model":"think","messages":[],"temperature":0.3}`)

	if backend.reloads.Load() != 1 {
		t.Fatalf("variants differing only in sampler defaults reloaded %d times, want one physical load", backend.reloads.Load())
	}
	requests := captured.all()
	if len(requests) != 3 {
		t.Fatalf("backend received %d requests", len(requests))
	}
	if requests[0]["temperature"] != 0.7 || requests[1]["temperature"] != 1.0 || requests[1]["top_p"] != 0.95 {
		t.Fatalf("requests = %v, want each variant's own defaults", requests)
	}
	if requests[2]["temperature"] != 0.3 || requests[2]["top_p"] != 0.95 {
		t.Fatalf("request = %v, want the client's temperature kept and the missing top_p filled", requests[2])
	}
}

func TestVariantsWithoutJinjaKwargsShareOneRuntime(t *testing.T) {
	service, backend := newTestServiceWithRequestProfiles(t, http.NotFoundHandler(), map[string]string{
		"cold": `{"model_param":"model.gguf","gendefaults":{"temperature":0.2},"router_unload_policy":"all"}`,
		"warm": `{"model_param":"model.gguf","gendefaults":{"temperature":1.1}}`,
	}, catalog.RequestProfilePolicy{InjectGenerationDefaults: true, ShareRuntimeByLaunchArguments: true, DefaultBackendMode: "llama_sdcpp"})
	captured := &capturedBackendRequests{}
	service.client = chatTemplateTestClient(t, captured.record(t))

	postChatTemplateRequest(t, service, `{"model":"cold","messages":[]}`)
	postChatTemplateRequest(t, service, `{"model":"warm","messages":[]}`)

	if backend.reloads.Load() != 1 {
		t.Fatalf("reloads = %d, want one physical load", backend.reloads.Load())
	}
	if requests := captured.all(); requests[0]["temperature"] != 0.2 || requests[1]["temperature"] != 1.1 {
		t.Fatalf("requests = %v, want each variant's temperature", requests)
	}
}

func TestSamplerVariantsReloadWhenBothSettingsAreOff(t *testing.T) {
	service, backend, captured := serviceWithSamplerVariants(t, catalog.RequestProfilePolicy{DefaultBackendMode: "kobold"})

	postChatTemplateRequest(t, service, `{"model":"instruct","messages":[]}`)
	postChatTemplateRequest(t, service, `{"model":"think","messages":[]}`)

	if backend.reloads.Load() != 2 {
		t.Fatalf("reloads = %d, want a reload per variant with the legacy fingerprint", backend.reloads.Load())
	}
	for _, request := range captured.all() {
		if _, injected := request["temperature"]; injected {
			t.Fatalf("request = %v, want the backend left to apply its own defaults", request)
		}
	}
}

func TestInjectedDefaultsWithoutLaunchArgumentSharingStillReload(t *testing.T) {
	service, backend, captured := serviceWithSamplerVariants(t, catalog.RequestProfilePolicy{InjectGenerationDefaults: true, DefaultBackendMode: "kobold"})

	postChatTemplateRequest(t, service, `{"model":"instruct","messages":[]}`)
	postChatTemplateRequest(t, service, `{"model":"think","messages":[]}`)

	if backend.reloads.Load() != 2 {
		t.Fatalf("reloads = %d, want the whole-file fingerprint to keep the variants apart", backend.reloads.Load())
	}
	if requests := captured.all(); requests[1]["temperature"] != 1.0 {
		t.Fatalf("requests = %v, want defaults injected regardless", requests)
	}
}

func TestOllamaChatDefaultsGoWhereEachBackendReadsThem(t *testing.T) {
	config := []byte(`{"model_param":"model.gguf","gendefaults":{"temperature":0.3}}`)
	for family, wantOptions := range map[string]bool{"kobold": false, "llama_sdcpp": true} {
		profile := catalog.ChatTemplateProfileForConfig(config, catalog.RequestProfilePolicy{InjectGenerationDefaults: true, DefaultBackendMode: family})

		rewrite := generationDefaultsForRequest(pathOllamaChat, profile)

		if rewrite == nil || rewrite.IntoOllamaOptions != wantOptions {
			t.Fatalf("%s: rewrite = %+v, want defaults in options = %v", family, rewrite, wantOptions)
		}
	}
}

func TestOllamaGenerateGetsDefaultsInItsOptions(t *testing.T) {
	service, _, captured := serviceWithSamplerVariants(t, catalog.RequestProfilePolicy{InjectGenerationDefaults: true, ShareRuntimeByLaunchArguments: true, DefaultBackendMode: "kobold"})

	postToPath(t, service, pathOllamaGenerate, `{"model":"think","prompt":"hi","options":{"num_ctx":4096}}`)

	requests := captured.all()
	options, ok := requests[0]["options"].(map[string]any)
	if !ok || options["temperature"] != 1.0 || options["num_ctx"] != float64(4096) {
		t.Fatalf("request = %v, want the defaults merged into options", requests[0])
	}
}
