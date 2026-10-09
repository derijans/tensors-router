package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLlamaSDCPPRoutesTextAndImagesToSeparateBackends(t *testing.T) {
	var textPosts atomic.Int32
	var imagePosts atomic.Int32
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		countingSplitBackend(t, "/v1/models", `{"object":"list","data":[{"id":"backend"}]}`, "/v1/chat/completions", `{"model":"backend","choices":[{"message":{"content":"ok"}}]}`, &textPosts),
		countingSplitBackend(t, "/sdapi/v1/sd-models", `[{"model_name":"ready"}]`, "/v1/images/generations", `{"model":"backend","data":[]}`, &imagePosts),
		map[string]string{
			"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\dream.safetensors"}`,
		},
	)

	listRecorder := httptest.NewRecorder()
	service.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil))
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"model_name":"combo-dream"`) {
		t.Fatalf("combined image was not listed in split mode: status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	expectProxyStatus(t, service, modelJSONRequest("/v1/chat/completions", `{"model":"combo","messages":[]}`, ""), http.StatusOK, "text")
	expectProxyStatus(t, service, modelJSONRequest("/v1/images/generations", `{"model":"combo-dream","prompt":"cat"}`, ""), http.StatusOK, "image")

	if textPosts.Load() != 1 || imagePosts.Load() != 1 {
		t.Fatalf("unexpected forward counts text=%d image=%d", textPosts.Load(), imagePosts.Load())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 1 {
		t.Fatalf("unexpected reload counts text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func countingSplitBackend(t *testing.T, readinessPath string, readinessBody string, workPath string, workBody string, posts *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == readinessPath {
			_, _ = w.Write([]byte(readinessBody))
			return
		}
		if r.URL.Path != workPath {
			t.Fatalf("unexpected path %s, want %s", r.URL.Path, workPath)
		}
		posts.Add(1)
		_, _ = w.Write([]byte(workBody))
	}
}
