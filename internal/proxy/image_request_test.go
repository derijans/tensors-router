package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/catalog"
)

func TestImageModelsUsePerModelBackendModeVisibility(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"kobold-combo": `{"model_param":"text.gguf","sdmodel":"/models/dream.safetensors","backend_mode":"kobold"}`,
		"native-combo": `{"model_param":"text.gguf","sdmodel":"/models/neon.safetensors","backend_mode":"llama_sdcpp"}`,
	})

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "native-combo-neon") {
		t.Fatalf("llama/sd.cpp combined image should be visible: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "kobold-combo-dream") {
		t.Fatalf("inactive kobold combined image should not be visible: %s", recorder.Body.String())
	}
}

func TestImageOnlyConfigIsNotCoreModel(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"image","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestImageModelListUsesImageEndpoints(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ready"}}]}`))
	}), map[string]string{
		"llm":   `{}`,
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})

	llmRecorder := httptest.NewRecorder()
	llmRequest := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	service.ServeHTTP(llmRecorder, llmRequest)

	if llmRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected llm status %d", llmRecorder.Code)
	}
	if strings.Contains(llmRecorder.Body.String(), `"id":"image"`) {
		t.Fatalf("image-only config should not be in /v1/models: %s", llmRecorder.Body.String())
	}
	if !strings.Contains(llmRecorder.Body.String(), `"id":"combo"`) || !strings.Contains(llmRecorder.Body.String(), `"id":"llm"`) {
		t.Fatalf("llm list missing expected models: %s", llmRecorder.Body.String())
	}

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(imageRecorder, imageRequest)

	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected image status %d", imageRecorder.Code)
	}
	if !strings.Contains(imageRecorder.Body.String(), `"model_name":"image-dream"`) {
		t.Fatalf("image list missing image-only model: %s", imageRecorder.Body.String())
	}
	if strings.Contains(imageRecorder.Body.String(), "combo-vision") {
		t.Fatalf("inactive combined image should not be listed: %s", imageRecorder.Body.String())
	}

	chatRecorder := httptest.NewRecorder()
	chatRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"combo","messages":[]}`))
	chatRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(chatRecorder, chatRequest)
	if chatRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected chat status %d body %s", chatRecorder.Code, chatRecorder.Body.String())
	}

	activeRecorder := httptest.NewRecorder()
	activeRequest := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(activeRecorder, activeRequest)

	if !strings.Contains(activeRecorder.Body.String(), `"model_name":"combo-vision"`) {
		t.Fatalf("active combined image should be listed: %s", activeRecorder.Body.String())
	}
}

func TestImageRequestLoadsImageOnlyConfig(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","data":[]}`))
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-dream","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.lastReload != "image.kcpps" {
		t.Fatalf("unexpected reload config %q", backend.lastReload)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"image-dream"`) {
		t.Fatalf("image response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestImageDiscoveryEndpointsForwardWithoutModel(t *testing.T) {
	endpoints := []string{"/sdapi/v1/loras", "/sdapi/v1/upscalers", "/sdapi/v1/schedulers", "/sdapi/v1/progress", "/sdapi/v1/get_last.json", "/sdcpp/v1/capabilities"}
	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			var sawImageBackend bool
			service, _, imageBackend := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("text backend should not receive image discovery path %s", r.URL.Path)
			}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != endpoint {
					t.Fatalf("unexpected image path %s", r.URL.Path)
				}
				sawImageBackend = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}), map[string]string{})

			recorder := httptest.NewRecorder()
			service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, endpoint, nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !sawImageBackend {
				t.Fatalf("image backend did not receive discovery request")
			}
			if imageBackend.reloads.Load() != 0 {
				t.Fatalf("discovery should not reload image config, got %d", imageBackend.reloads.Load())
			}
		})
	}
}

func TestInactiveCombinedImageModelIsRejected(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-vision","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestActiveCombinedImageModelDoesNotReload(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/chat/completions" {
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ready"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
	}), map[string]string{
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})

	chatRecorder := httptest.NewRecorder()
	chatRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"combo","messages":[]}`))
	chatRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(chatRecorder, chatRequest)

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-vision","prompt":"cat"}`))
	imageRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(imageRecorder, imageRequest)

	if chatRecorder.Code != http.StatusOK || imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected statuses chat=%d image=%d imageBody=%s", chatRecorder.Code, imageRecorder.Code, imageRecorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload for combined config, got %d", backend.reloads.Load())
	}
}

func TestImageConfigSwitchWaitsForStreamingLLMRequest(t *testing.T) {
	chatStarted := make(chan struct{})
	releaseChat := make(chan struct{})
	imageReloaded := make(chan struct{})
	var imageReloadOnce sync.Once

	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"backend\",\"choices\":[]}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(chatStarted)
			<-releaseChat
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
		case koboldPerfPath:
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}), map[string]string{
		"llm":   `{}`,
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	backend.onReload = func(filename string) {
		if filename == "image.kcpps" {
			imageReloadOnce.Do(func() {
				close(imageReloaded)
			})
		}
	}

	chatDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		close(chatDone)
	}()

	select {
	case <-chatStarted:
	case <-time.After(time.Second):
		t.Fatal("chat did not start")
	}

	imageDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-dream","prompt":"cat"}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("unexpected image status %d body %s", recorder.Code, recorder.Body.String())
		}
		close(imageDone)
	}()

	select {
	case <-imageReloaded:
		t.Fatal("image config reloaded while llm stream was active")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseChat)

	select {
	case <-imageDone:
	case <-time.After(time.Second):
		t.Fatal("image request did not finish")
	}
	select {
	case <-chatDone:
	case <-time.After(time.Second):
		t.Fatal("chat request did not finish")
	}
	if backend.reloads.Load() != 2 {
		t.Fatalf("expected llm and image reloads, got %d", backend.reloads.Load())
	}
}

func TestSdcppJobsRouteBackToSubmittingImageConfig(t *testing.T) {
	assertSdcppJobRoutesToSubmittingConfig(t, http.MethodGet, "/sdcpp/v1/jobs/job-a", `{"id":"job-a","status":"done"}`)
}

func TestSdcppJobCancelRoutesBackToSubmittingImageConfig(t *testing.T) {
	assertSdcppJobRoutesToSubmittingConfig(t, http.MethodPost, "/sdcpp/v1/jobs/job-a/cancel", `{"id":"job-a","status":"cancelled"}`)
}

func assertSdcppJobRoutesToSubmittingConfig(t *testing.T, jobMethod string, jobPath string, jobResponse string) {
	var imageBackend *fakeBackend
	textHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("text backend should not receive image path %s", r.URL.Path)
	})
	service, _, createdImageBackend := newSplitTestServiceWithConfigContents(t, textHandler, sdcppJobImageHandler(t, &imageBackend, jobPath, jobResponse), map[string]string{
		"first":  `{"nomodel":true,"sdmodel":"C:\\models\\first.safetensors"}`,
		"second": `{"nomodel":true,"sdmodel":"C:\\models\\second.safetensors"}`,
	})
	imageBackend = createdImageBackend
	useTinyTransportLimits(service)

	expectProxyStatus(t, service, modelJSONRequest("/sdcpp/v1/img_gen", `{"model":"first-first","prompt":"cat"}`, "first-first"), http.StatusOK, "submit")
	expectProxyStatus(t, service, modelJSONRequest("/v1/images/generations", `{"model":"second-second","prompt":"dog"}`, "second-second"), http.StatusOK, "switch")
	expectProxyStatus(t, service, httptest.NewRequest(jobMethod, jobPath, nil), http.StatusOK, "job")
	if imageBackend.lastReload != "first.kcpps" {
		t.Fatalf("job route did not restore original config, got %q", imageBackend.lastReload)
	}
}

func sdcppJobImageHandler(t *testing.T, imageBackend **fakeBackend, jobPath string, jobResponse string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/sdcpp/v1/img_gen":
			requireBodyContains(t, body, `"model":"first-first"`, "job submit used wrong image model")
			_, _ = w.Write([]byte(`{"job_id":"job-a"}`))
		case "/v1/images/generations":
			requireBodyContains(t, body, `"model":"second-second"`, "switch request used wrong image model")
			_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
		case jobPath:
			if *imageBackend == nil || (*imageBackend).lastReload != "first.kcpps" {
				t.Fatalf("job request did not reload original config")
			}
			_, _ = w.Write([]byte(jobResponse))
		default:
			t.Fatalf("unexpected image path %s", r.URL.Path)
		}
	}
}

func requireBodyContains(t *testing.T, body []byte, fragment string, failure string) {
	if !strings.Contains(string(body), fragment) {
		t.Fatalf("%s: %s", failure, string(body))
	}
}

func TestColdImageRequestWaitsForImageModelsEndpoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int32
	var generations atomic.Int32
	server := httptest.NewServer(coldImageBackendHandler(t, &probes, &generations))
	defer server.Close()

	backend := &fakeBackend{url: mustParseURL(t, server.URL), healthy: true}
	service := NewService(ServiceConfig{
		Backend:   backend,
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 0

	expectProxyStatus(t, service, modelJSONRequest("/v1/images/generations", `{"model":"image-dream","prompt":"cat"}`, ""), http.StatusOK, "image generation")
	if probes.Load() != 3 {
		t.Fatalf("expected three image readiness probes, got %d", probes.Load())
	}
	if generations.Load() != 1 {
		t.Fatalf("expected one image generation request, got %d", generations.Load())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
}

func coldImageBackendHandler(t *testing.T, probes *atomic.Int32, generations *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models":
			if probes.Add(1) < 3 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"model_name":"dream"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/generations":
			if probes.Load() < 3 {
				t.Fatalf("image request forwarded before image models endpoint was ready")
			}
			generations.Add(1)
			_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","data":[]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}
}
