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
	"testing"

	"tensors-router/internal/catalog"
)

var sdServerModelManagementRequests = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/sdcpp/v1/models/load"},
	{http.MethodPost, "/sdcpp/v1/models/unload"},
	{http.MethodPost, "/sdcpp/v1/models/refresh"},
	{http.MethodGet, "/sdcpp/v1/models/status"},
	{http.MethodGet, "/sdcpp/v1/model-files"},
	{http.MethodGet, "/sdcpp/v1/recipes"},
	{http.MethodGet, "/sdcpp/v1/recipes/"},
}

func TestSDServerModelManagementIsNeverForwardedToTheLoadedImageRuntime(t *testing.T) {
	service, forwarded := newLoadedSDServerTestService(t)

	for _, request := range sdServerModelManagementRequests {
		for _, path := range []string{request.path, "/router/webuis/sdcpp" + request.path} {
			recorder := httptest.NewRecorder()
			service.ServeHTTP(recorder, httptest.NewRequest(request.method, path, strings.NewReader(`{"recipe":"other"}`)))
			if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "not_found") {
				t.Fatalf("%s %s: status=%d body=%s, want 404 not_found", request.method, path, recorder.Code, recorder.Body.String())
			}
		}
	}
	if paths := forwarded(); len(paths) != 0 {
		t.Fatalf("sd-server model management reached the backend behind the lane core: %v", paths)
	}
}

func TestSDServerImageRoutesStillReachTheLoadedImageRuntime(t *testing.T) {
	service, forwarded := newLoadedSDServerTestService(t)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sdcpp/v1/capabilities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("capabilities status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if paths := forwarded(); len(paths) != 1 || paths[0] != "/sdcpp/v1/capabilities" {
		t.Fatalf("forwarded paths = %v, want the capabilities request", paths)
	}
}

func TestSDServerUpscaleRunsOnTheLoadedImageRuntimeAndCountsAsImageWork(t *testing.T) {
	service, forwarded := newLoadedSDServerTestService(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, sdcppUpscalePath, strings.NewReader(`{"image":"aW1hZ2U=","upscale_factor":2}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upscale status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if paths := forwarded(); len(paths) != 1 || paths[0] != sdcppUpscalePath {
		t.Fatalf("forwarded paths = %v, want the upscale request", paths)
	}
	if !isImageGenerationPath(sdcppUpscalePath) {
		t.Fatal("sd-server upscale through the WebUI proxy would be counted as text analytics")
	}
}

func newLoadedSDServerTestService(t *testing.T) (*Service, func() []string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{"backend_mode":"llama_sdcpp","nomodel":true,"sdmodel":"dream.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var mutex sync.Mutex
	var forwardedPaths []string
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
			return
		}
		mutex.Lock()
		forwardedPaths = append(forwardedPaths, r.URL.Path)
		mutex.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(backendServer.Close)

	backend := &fakeBackend{url: mustParseURL(t, backendServer.URL), healthy: true}
	service := NewService(ServiceConfig{
		TextBackend:  backend,
		ImageBackend: backend,
		BackendMode:  BackendModeLlamaSDCPP,
		Catalog:      catalog.New(dir),
		Logger:       log.New(io.Discard, "", 0),
	})
	loadWebUIForTest(t, service, "sdcpp", "", "image-dream")
	service.webUI.session.set("sdcpp", true)
	return service, func() []string {
		mutex.Lock()
		defer mutex.Unlock()
		return append([]string(nil), forwardedPaths...)
	}
}
