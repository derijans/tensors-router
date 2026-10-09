package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLlamaSDCPPRouterLoadCombinedConfigLoadsBothLanes(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
				return
			}
			t.Fatalf("unexpected text path %s", r.URL.Path)
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
				return
			}
			t.Fatalf("unexpected image path %s", r.URL.Path)
		}),
		map[string]string{
			"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\dream.safetensors"}`,
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader(`{"model":"combo"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 1 {
		t.Fatalf("expected both lanes to reload, got text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}
