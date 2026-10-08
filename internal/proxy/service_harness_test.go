package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

type fakeBackend struct {
	url        *url.URL
	reloads    atomic.Int32
	restarts   atomic.Int32
	unloads    atomic.Int32
	healthy    bool
	lastReload string
	onReload   func(string)
	onRestart  func()
	onUnload   func()
	reloadErr  func(string) error
	restartErr func() error
}

func (backend *fakeBackend) URL() *url.URL {
	copyValue := *backend.url
	return &copyValue
}

func (backend *fakeBackend) ReloadConfig(ctx context.Context, filename string) error {
	backend.lastReload = filename
	backend.reloads.Add(1)
	if backend.onReload != nil {
		backend.onReload(filename)
	}
	if backend.reloadErr != nil {
		return backend.reloadErr(filename)
	}
	return nil
}

func (backend *fakeBackend) Restart(ctx context.Context) error {
	backend.restarts.Add(1)
	if backend.onRestart != nil {
		backend.onRestart()
	}
	if backend.restartErr != nil {
		return backend.restartErr()
	}
	return nil
}

func (backend *fakeBackend) Unload(ctx context.Context) error {
	backend.unloads.Add(1)
	if backend.onUnload != nil {
		backend.onUnload()
	}
	return nil
}

func (backend *fakeBackend) Healthy(ctx context.Context) bool {
	return backend.healthy
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func testHTTPResponse(status int, contentType string, body string) *http.Response {
	header := http.Header{}
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func readyTextHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
			return
		}
		t.Fatalf("unexpected text path %s", r.URL.Path)
	}
}

func readyImageHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
			return
		}
		t.Fatalf("unexpected image path %s", r.URL.Path)
	}
}

func newTestService(t *testing.T, backendHandler http.Handler) (*Service, *fakeBackend) {
	return newTestServiceWithLogger(t, backendHandler, log.New(io.Discard, "", 0))
}

func writeProxyTestConfig(t *testing.T, dir string, id string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".kcpps"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newReadyFakeBackend(t *testing.T, onReload func(string)) *fakeBackend {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		case "/sdapi/v1/sd-models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeBackend{url: backendURL, healthy: true, onReload: onReload}
}

func eventsSnapshot(mu *sync.Mutex, events *[]string) []string {
	mu.Lock()
	defer mu.Unlock()
	return append([]string{}, (*events)...)
}

func eventBefore(events []string, first string, second string) bool {
	firstIndex := -1
	secondIndex := -1
	for index, event := range events {
		if event == first && firstIndex == -1 {
			firstIndex = index
		}
		if event == second && secondIndex == -1 {
			secondIndex = index
		}
	}
	return firstIndex >= 0 && secondIndex >= 0 && firstIndex < secondIndex
}

func newTestServiceWithModels(t *testing.T, backendHandler http.Handler, modelIDs ...string) (*Service, *fakeBackend) {
	return newTestServiceWithModelsAndLogger(t, backendHandler, log.New(io.Discard, "", 0), modelIDs...)
}

func newTestServiceWithLogger(t *testing.T, backendHandler http.Handler, logger *log.Logger) (*Service, *fakeBackend) {
	return newTestServiceWithModelsAndLogger(t, backendHandler, logger, "a")
}

func newTestServiceWithModelsAndLogger(t *testing.T, backendHandler http.Handler, logger *log.Logger, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	return newTestServiceWithBackendSetup(t, backendHandler, logger, true, modelIDs...)
}

func newTestServiceWithRegistry(t *testing.T, registry *cluster.Registry, backendHandler http.Handler, token string) (*Service, *fakeBackend) {
	t.Helper()

	server := httptest.NewServer(backendHandler)
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend:      backend,
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterToken: token,
		Logger:       log.New(io.Discard, "", 0),
	})
	return service, backend
}

func newTestServiceWithRawBackend(t *testing.T, backendHandler http.Handler, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	return newTestServiceWithBackendSetup(t, backendHandler, log.New(io.Discard, "", 0), false, modelIDs...)
}

func newTestServiceWithConfigContents(t *testing.T, backendHandler http.Handler, configs map[string]string) (*Service, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for modelID, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
			return
		}
		backendHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend: backend,
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})
	return service, backend
}

func newSplitTestServiceWithConfigContents(t *testing.T, textHandler http.Handler, imageHandler http.Handler, configs map[string]string) (*Service, *fakeBackend, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for modelID, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	textServer := httptest.NewServer(textHandler)
	t.Cleanup(textServer.Close)
	imageServer := httptest.NewServer(imageHandler)
	t.Cleanup(imageServer.Close)
	textURL, err := url.Parse(textServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageURL, err := url.Parse(imageServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	textBackend := &fakeBackend{url: textURL, healthy: true}
	imageBackend := &fakeBackend{url: imageURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend:      textBackend,
		TextBackend:  textBackend,
		ImageBackend: imageBackend,
		BackendMode:  BackendModeLlamaSDCPP,
		Catalog:      catalog.New(dir),
		ConfigDir:    dir,
		Logger:       log.New(io.Discard, "", 0),
	})
	return service, textBackend, imageBackend
}

func postProxyModelRequest(t *testing.T, service *Service, path string, body string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status path=%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
	}
}

func newTestServiceWithBackendSetup(t *testing.T, backendHandler http.Handler, logger *log.Logger, addModelsEndpoint bool, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for _, modelID := range modelIDs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if addModelsEndpoint && r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		backendHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend: backend,
		Catalog: catalog.New(dir),
		Logger:  logger,
	})
	return service, backend
}
