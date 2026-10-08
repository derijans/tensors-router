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
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
				return
			}
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected text path %s", r.URL.Path)
			}
			textPosts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
				return
			}
			if r.URL.Path != "/v1/images/generations" {
				t.Fatalf("unexpected image path %s", r.URL.Path)
			}
			imagePosts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
		}),
		map[string]string{
			"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\dream.safetensors"}`,
		},
	)

	listRecorder := httptest.NewRecorder()
	service.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil))
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"model_name":"combo-dream"`) {
		t.Fatalf("combined image was not listed in split mode: status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	textRecorder := httptest.NewRecorder()
	textRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"combo","messages":[]}`))
	textRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(textRecorder, textRequest)
	if textRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected text status %d body %s", textRecorder.Code, textRecorder.Body.String())
	}

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-dream","prompt":"cat"}`))
	imageRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(imageRecorder, imageRequest)
	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected image status %d body %s", imageRecorder.Code, imageRecorder.Body.String())
	}

	if textPosts.Load() != 1 || imagePosts.Load() != 1 {
		t.Fatalf("unexpected forward counts text=%d image=%d", textPosts.Load(), imagePosts.Load())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 1 {
		t.Fatalf("unexpected reload counts text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

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
