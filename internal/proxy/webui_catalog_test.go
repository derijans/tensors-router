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

func TestSiteWebUICatalogBuildsCompatibleEntriesWithStableURLs(t *testing.T) {
	service := newWebUICatalogTestService(t, map[string]string{
		"text":        `{"model_param":"text.gguf"}`,
		"image":       `{"nomodel":true,"sdmodel":"dream.safetensors"}`,
		"music":       `{"nomodel":true,"musicllm":"music.gguf"}`,
		"split-text":  `{"backend_mode":"llama_sdcpp","model_param":"split.gguf"}`,
		"split-image": `{"backend_mode":"llama_sdcpp","nomodel":true,"sdmodel":"split.safetensors"}`,
	})

	response := requestWebUICatalog(t, service, "/router/v1/site/webuis", "")
	entries := webUIEntriesByID(response.Data)
	expectedURLs := map[string]string{
		"kobold-lite":  "/router/webuis/kobold-lite/",
		"kobold-lcpp":  "/router/webuis/kobold-lcpp/",
		"kobold-sd":    "/router/webuis/kobold-sd/",
		"kobold-music": "/router/webuis/kobold-music/",
		"llama":        "/router/webuis/llama/",
		"sdcpp":        "/router/webuis/sdcpp/",
	}
	for id, expectedURL := range expectedURLs {
		if entries[id].URL != expectedURL {
			t.Fatalf("unexpected url for %s: %q", id, entries[id].URL)
		}
	}
	if len(entries["kobold-sd"].CompatibleModels) != 1 || entries["kobold-sd"].CompatibleModels[0].ImageID != "image-dream" {
		t.Fatalf("unexpected stable compatible models %#v", entries["kobold-sd"].CompatibleModels)
	}
	if len(entries["kobold-music"].CompatibleModels) != 1 || entries["kobold-music"].CompatibleModels[0].ModelID != "music" {
		t.Fatalf("unexpected music compatible models %#v", entries["kobold-music"].CompatibleModels)
	}
}

func TestWebUISessionToggleIsStoredInRouterMemory(t *testing.T) {
	service := newWebUICatalogTestService(t, map[string]string{
		"text": `{"model_param":"text.gguf"}`,
	})

	catalogBefore := requestWebUICatalog(t, service, "/router/v1/site/webuis", "")
	if webUIEntryEnabled(t, catalogBefore, "kobold-lite") {
		t.Fatalf("toggle should default off")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/webuis/session", strings.NewReader(`{"id":"kobold-lite","enabled":true}`))
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected session status %d body %s", recorder.Code, recorder.Body.String())
	}
	var toggled WebUICatalogResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &toggled); err != nil {
		t.Fatal(err)
	}
	if !webUIEntryEnabled(t, toggled, "kobold-lite") {
		t.Fatalf("toggle should be on after session update")
	}

	restarted := newWebUICatalogTestService(t, map[string]string{
		"text": `{"model_param":"text.gguf"}`,
	})
	catalogAfterRestart := requestWebUICatalog(t, restarted, "/router/v1/site/webuis", "")
	if webUIEntryEnabled(t, catalogAfterRestart, "kobold-lite") {
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
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})

	recorder := httptest.NewRecorder()
	body := `{"id":"kobold-sd","model_id":"image","image_id":"image-dream"}`
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
						ID:                  "kobold-lite",
						Name:                "KoboldCpp Lite",
						Backend:             "koboldcpp",
						BackendMode:         BackendModeKobold,
						Lane:                cluster.RouteLaneText,
						URL:                 "/router/webuis/kobold-lite/",
						NodeID:              "slave-a",
						RequiresLoadedModel: true,
						CompatibleModels: []WebUICompatibleModel{
							{ID: "remote-text", ModelID: "remote-text", LocalID: "remote-text", Filename: "remote-text.kcpps", NodeID: "slave-a", NodeURL: remoteURL(r), BackendMode: BackendModeKobold},
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
			_ = json.NewEncoder(w).Encode(webUILoadResponse{OK: true, ID: "kobold-lite", URL: "/router/webuis/kobold-lite/", ModelID: "remote-text"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterModel("remote-text", "slave-a", "remote-hash", "remote-config", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}
	backendURL := mustParseURL(t, "http://127.0.0.1:1")
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		ClusterToken: "secret",
		SlaveURLs:    []string{remote.URL},
		Logger:       log.New(io.Discard, "", 0),
	})

	recorder := httptest.NewRecorder()
	body := `{"id":"kobold-lite","model_id":"remote-text"}`
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/webuis/load", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", recorder.Code, recorder.Body.String())
	}
	if loadSeen.Load() != 1 {
		t.Fatalf("expected remote load")
	}
}

func TestLocalWebUIProxyStripsStablePrefixPreservesQueryAndRewritesRedirect(t *testing.T) {
	service := newProxyReadyWebUIService(t)
	loadWebUIForTest(t, service, "kobold-lcpp", "text", "")
	service.webUISession.set("kobold-lcpp", true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lcpp/assets/app.js?theme=dark", nil)
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "local:/lcpp/assets/app.js?theme=dark" {
		t.Fatalf("unexpected proxy body %q", recorder.Body.String())
	}

	redirect := httptest.NewRecorder()
	service.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lcpp/redirect", nil))
	if redirect.Code != http.StatusFound {
		t.Fatalf("unexpected redirect status %d body %s", redirect.Code, redirect.Body.String())
	}
	if location := redirect.Header().Get("Location"); location != "/router/webuis/kobold-lcpp/next?x=1" {
		t.Fatalf("unexpected redirect location %q", location)
	}
}

func TestRemoteWebUIProxyUsesSlaveTokenAndRewritesNodeRedirect(t *testing.T) {
	var sawToken atomic.Bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer secret" {
			sawToken.Store(true)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/webuis":
			writeRemoteActiveWebUI(t, w, remoteURL(r), "slave-a")
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/webuis/kobold-lite/panel":
			if r.URL.RawQuery != "tab=1" {
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			w.Header().Set("Location", "/router/v1/node/webuis/kobold-lite/next?x=1")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: mustParseURL(t, "http://127.0.0.1:1"), healthy: false},
		Catalog:      catalog.New(t.TempDir()),
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		ClusterToken: "secret",
		SlaveURLs:    []string{remote.URL},
		Logger:       log.New(io.Discard, "", 0),
	})
	service.webUISession.set("kobold-lite", true)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lite/panel?tab=1", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("unexpected remote status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawToken.Load() {
		t.Fatalf("expected slave authorization token")
	}
	if location := recorder.Header().Get("Location"); location != "/router/webuis/kobold-lite/next?x=1" {
		t.Fatalf("unexpected remote redirect location %q", location)
	}
}

func TestWebUIProxyReturnsUnavailableWithoutActiveBackend(t *testing.T) {
	service := newWebUICatalogTestService(t, map[string]string{
		"text": `{"model_param":"text.gguf"}`,
	})
	service.webUISession.set("kobold-lite", true)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lite/", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable, got %d body %s", recorder.Code, recorder.Body.String())
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
	koboldBackend := &fakeBackend{url: mustParseURL(t, "http://127.0.0.1:1"), healthy: true}
	llamaBackend := &fakeBackend{url: mustParseURL(t, "http://127.0.0.1:2"), healthy: true}
	sdcppBackend := &fakeBackend{url: mustParseURL(t, "http://127.0.0.1:3"), healthy: true}
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
		Catalog: catalog.New(dir),
		NodeID:  "local",
		Logger:  log.New(io.Discard, "", 0),
	})
}

func newProxyReadyWebUIService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "text.kcpps"), []byte(`{"model_param":"text.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"ready"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lcpp/redirect":
			w.Header().Set("Location", "/lcpp/next?x=1")
			w.WriteHeader(http.StatusFound)
		default:
			_, _ = w.Write([]byte("local:" + r.URL.Path + "?" + r.URL.RawQuery))
		}
	}))
	t.Cleanup(backendServer.Close)
	backend := &fakeBackend{url: mustParseURL(t, backendServer.URL), healthy: true}
	return NewService(ServiceConfig{
		Backend: backend,
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})
}

func loadWebUIForTest(t *testing.T, service *Service, id string, modelID string, imageID string) {
	t.Helper()
	body := `{"id":"` + id + `","model_id":"` + modelID + `","image_id":"` + imageID + `"}`
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/webuis/load", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", recorder.Code, recorder.Body.String())
	}
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

func writeRemoteActiveWebUI(t *testing.T, w http.ResponseWriter, nodeURL string, nodeID string) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(WebUICatalogResponse{
		Object: "list",
		Data: []WebUIEntry{
			{
				ID:          "kobold-lite",
				Name:        "KoboldCpp Lite",
				Backend:     "koboldcpp",
				BackendMode: BackendModeKobold,
				Lane:        cluster.RouteLaneText,
				URL:         "/router/webuis/kobold-lite/",
				NodeID:      nodeID,
				NodeURL:     nodeURL,
				Active:      true,
				CompatibleModels: []WebUICompatibleModel{
					{ID: "remote-text", ModelID: "remote-text", LocalID: "remote-text", Filename: "remote-text.kcpps", NodeID: nodeID, NodeURL: nodeURL, BackendMode: BackendModeKobold, Active: true},
				},
			},
		},
	})
}

func remoteURL(r *http.Request) string {
	return "http://" + r.Host
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
