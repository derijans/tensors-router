package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestSiteWebUICatalogBuildsCompatibleEntriesWithHostURLs(t *testing.T) {
	service := newWebUICatalogTestService(t, map[string]string{
		"text":        `{"model_param":"text.gguf"}`,
		"image":       `{"nomodel":true,"sdmodel":"dream.safetensors"}`,
		"music":       `{"nomodel":true,"musicllm":"music.gguf"}`,
		"split-text":  `{"backend_mode":"llama_sdcpp","model_param":"split.gguf"}`,
		"split-image": `{"backend_mode":"llama_sdcpp","nomodel":true,"sdmodel":"split.safetensors"}`,
	})

	response := requestWebUICatalog(t, service, "/router/v1/site/webuis", "")
	entries := webUIEntriesByID(response.Data)
	if entries["local:kobold-lite"].URL != "https://kobold.example.test/base/" {
		t.Fatalf("unexpected kobold lite url %q", entries["local:kobold-lite"].URL)
	}
	if entries["local:kobold-llama"].URL != "https://kobold.example.test/base/lcpp/" {
		t.Fatalf("unexpected kobold llama url %q", entries["local:kobold-llama"].URL)
	}
	if entries["local:kobold-stable"].URL != "https://kobold.example.test/base/sdui/" {
		t.Fatalf("unexpected kobold stable url %q", entries["local:kobold-stable"].URL)
	}
	if entries["local:llama-server"].URL != "https://llama.example.test/ui/" {
		t.Fatalf("unexpected llama url %q", entries["local:llama-server"].URL)
	}
	if entries["local:sd-server"].URL != "https://sd.example.test/ui/" {
		t.Fatalf("unexpected sd url %q", entries["local:sd-server"].URL)
	}
	if len(entries["local:kobold-stable"].CompatibleModels) != 1 || entries["local:kobold-stable"].CompatibleModels[0].ImageID != "image-dream" {
		t.Fatalf("unexpected stable compatible models %#v", entries["local:kobold-stable"].CompatibleModels)
	}
	if len(entries["local:kobold-music"].CompatibleModels) != 1 || entries["local:kobold-music"].CompatibleModels[0].ModelID != "music" {
		t.Fatalf("unexpected music compatible models %#v", entries["local:kobold-music"].CompatibleModels)
	}
}

func TestWebUISessionToggleIsStoredInRouterMemory(t *testing.T) {
	service := newWebUICatalogTestService(t, map[string]string{
		"text": `{"model_param":"text.gguf"}`,
	})

	catalogBefore := requestWebUICatalog(t, service, "/router/v1/site/webuis", "")
	if webUIEntryEnabled(t, catalogBefore, "local:kobold-lite") {
		t.Fatalf("toggle should default off")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/webuis/session", strings.NewReader(`{"id":"local:kobold-lite","enabled":true}`))
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected session status %d body %s", recorder.Code, recorder.Body.String())
	}
	var toggled WebUICatalogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &toggled); err != nil {
		t.Fatal(err)
	}
	if !webUIEntryEnabled(t, toggled, "local:kobold-lite") {
		t.Fatalf("toggle should be on after session update")
	}

	restarted := newWebUICatalogTestService(t, map[string]string{
		"text": `{"model_param":"text.gguf"}`,
	})
	catalogAfterRestart := requestWebUICatalog(t, restarted, "/router/v1/site/webuis", "")
	if webUIEntryEnabled(t, catalogAfterRestart, "local:kobold-lite") {
		t.Fatalf("toggle should reset when router service is recreated")
	}
}

func TestNodeSiteWebUIRequiresClusterToken(t *testing.T) {
	service := newWebUICatalogTestService(t, map[string]string{
		"text": `{"model_param":"text.gguf"}`,
	})
	service.clusterToken = "secret"

	unauthorized := httptest.NewRecorder()
	service.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/router/v1/node/site/webuis", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", unauthorized.Code)
	}

	authorized := requestWebUICatalog(t, service, "/router/v1/node/site/webuis", "secret")
	if len(authorized.Data) == 0 {
		t.Fatalf("expected webui entries")
	}
}

func TestSiteWebUILoadUsesImageReadinessForImageUI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{"nomodel":true,"sdmodel":"dream.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var textProbes atomic.Int64
	var imageProbes atomic.Int64
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			textProbes.Add(1)
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"ready"}]}`))
		case "/sdapi/v1/sd-models":
			imageProbes.Add(1)
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer backendServer.Close()

	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend: backend,
		WebUIHosts: BackendWebUIHosts{
			Kobold: backendServer.URL,
		},
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})

	recorder := httptest.NewRecorder()
	body := `{"id":"local:kobold-stable","model_id":"image","image_id":"image-dream"}`
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/webuis/load", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.lastReload != "image.kcpps" {
		t.Fatalf("unexpected reload %q", backend.lastReload)
	}
	if imageProbes.Load() == 0 || textProbes.Load() != 0 {
		t.Fatalf("expected image readiness only, text=%d image=%d", textProbes.Load(), imageProbes.Load())
	}
}

func TestMasterWebUILoadRoutesToSelectedRemoteNode(t *testing.T) {
	var loadSeen atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/webuis":
			_ = json.NewEncoder(w).Encode(WebUICatalogResponse{
				Object: "list",
				Data: []WebUIEntry{
					{
						ID:                  "slave-a:kobold-lite",
						Name:                "KoboldCpp Lite",
						Backend:             "koboldcpp",
						BackendMode:         BackendModeKobold,
						Lane:                cluster.RouteLaneText,
						URL:                 "https://slave.example.test/",
						NodeID:              "slave-a",
						RequiresLoadedModel: true,
						CompatibleModels: []WebUICompatibleModel{
							{ID: "remote-text", ModelID: "remote-text", Filename: "remote-text.kcpps", NodeID: "slave-a", BackendMode: BackendModeKobold},
						},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/webuis/load":
			loadSeen.Add(1)
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"model_id":"remote-text"`) {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(webUILoadResponse{OK: true, ID: "slave-a:kobold-lite", URL: "https://slave.example.test/", ModelID: "remote-text"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Catalog:      catalog.New(t.TempDir()),
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		ClusterToken: "secret",
		SlaveURLs:    []string{remote.URL},
		Logger:       log.New(io.Discard, "", 0),
	})

	recorder := httptest.NewRecorder()
	body := `{"id":"slave-a:kobold-lite","model_id":"remote-text"}`
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/webuis/load", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", recorder.Code, recorder.Body.String())
	}
	if loadSeen.Load() != 1 {
		t.Fatalf("expected remote load")
	}
}

func newWebUICatalogTestService(t *testing.T, configs map[string]string) *Service {
	t.Helper()
	dir := t.TempDir()
	for id, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, id+".kcpps"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	koboldURL := mustParseURL(t, "http://127.0.0.1:1")
	llamaURL := mustParseURL(t, "http://127.0.0.1:2")
	sdcppURL := mustParseURL(t, "http://127.0.0.1:3")
	koboldBackend := &fakeBackend{url: koboldURL, healthy: true}
	llamaBackend := &fakeBackend{url: llamaURL, healthy: true}
	sdcppBackend := &fakeBackend{url: sdcppURL, healthy: true}
	return NewService(ServiceConfig{
		BackendMode: BackendModeKobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold: {
				TextBackend:  koboldBackend,
				ImageBackend: koboldBackend,
			},
			BackendModeLlamaSDCPP: {
				TextBackend:  llamaBackend,
				ImageBackend: sdcppBackend,
			},
		},
		WebUIHosts: BackendWebUIHosts{
			Kobold: "https://kobold.example.test/base",
			Llama:  "https://llama.example.test/ui",
			SDCPP:  "https://sd.example.test/ui",
		},
		Catalog: catalog.New(dir),
		NodeID:  "local",
		Logger:  log.New(io.Discard, "", 0),
	})
}

func requestWebUICatalog(t *testing.T, service *Service, path string, token string) WebUICatalogResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected catalog status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response WebUICatalogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func webUIEntriesByID(entries []WebUIEntry) map[string]WebUIEntry {
	result := map[string]WebUIEntry{}
	for _, entry := range entries {
		result[entry.ID] = entry
	}
	return result
}

func webUIEntryEnabled(t *testing.T, response WebUICatalogResponse, id string) bool {
	t.Helper()
	entry, ok := webUIEntryByID(response.Data, id)
	if !ok {
		t.Fatalf("missing webui %q", id)
	}
	return entry.Enabled
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
