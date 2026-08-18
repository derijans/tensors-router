package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/modelstate"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
	"tensors-router/internal/siteapi"
)

func TestLocalModelStateValidatesPersistsAndRequiresClusterAuth(t *testing.T) {
	service, store, registry := newModelStateTestService(t)
	unknown := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "missing", Enabled: false}, "")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown model status=%d body=%s", unknown.Code, unknown.Body.String())
	}
	if disabled, err := store.Disabled(context.Background(), "missing"); err != nil || disabled {
		t.Fatalf("unknown model mutated state disabled=%t err=%v", disabled, err)
	}

	unauthorized := postModelState(service, "/router/v1/node/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("node state endpoint accepted missing cluster token status=%d", unauthorized.Code)
	}
	authorized := postModelState(service, "/router/v1/node/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "secret")
	if authorized.Code != http.StatusOK {
		t.Fatalf("node state update status=%d body=%s", authorized.Code, authorized.Body.String())
	}
	if !registry.ConfigDisabled("node-a", "model-a.kcpps") {
		t.Fatal("registry was not refreshed before the response")
	}
	if disabled, err := store.Disabled(context.Background(), "model-a"); err != nil || !disabled {
		t.Fatalf("disabled state was not persisted disabled=%t err=%v", disabled, err)
	}
	publicModels := httptest.NewRecorder()
	service.ServeHTTP(publicModels, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if publicModels.Code != http.StatusOK || strings.Contains(publicModels.Body.String(), "model-a") {
		t.Fatalf("disabled model remained in public list status=%d body=%s", publicModels.Code, publicModels.Body.String())
	}
	directLoad := httptest.NewRecorder()
	service.ServeHTTP(directLoad, httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader(`{"model":"model-a"}`)))
	if directLoad.Code == http.StatusOK {
		t.Fatalf("disabled model accepted direct load body=%s", directLoad.Body.String())
	}
	recipeStore, err := recipes.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := recipeStore.Save(recipes.Recipe{ID: "recipe-a", PublicID: "recipe-a", Text: &recipes.Component{Kind: recipes.KindText, NodeID: "node-a", ModelID: "model-a", ConfigFilename: "model-a.kcpps"}}, false); err != nil {
		t.Fatal(err)
	}
	service.recipeStore = recipeStore
	recipeRoute := httptest.NewRecorder()
	service.ServeHTTP(recipeRoute, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"recipe-a","messages":[]}`)))
	if recipeRoute.Code != http.StatusNotFound {
		t.Fatalf("disabled recipe component remained routable status=%d body=%s", recipeRoute.Code, recipeRoute.Body.String())
	}
	postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: true}, "")
	if registry.ConfigDisabled("node-a", "model-a.kcpps") {
		t.Fatal("idempotent enable did not refresh registry state")
	}
}

func TestDisabledModelRejectedOnInferencePath(t *testing.T) {
	service, _, registry := newModelStateTestService(t)
	backend := service.backend.(*fakeBackend)

	disabled := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	if !registry.ConfigDisabled("node-a", "model-a.kcpps") {
		t.Fatal("registry did not observe disabled model")
	}

	before := backend.reloads.Load()
	chat := httptest.NewRecorder()
	service.ServeHTTP(chat, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model-a","messages":[]}`)))
	if chat.Code != http.StatusNotFound {
		t.Fatalf("disabled model served on chat completions status=%d body=%s", chat.Code, chat.Body.String())
	}
	if got := backend.reloads.Load(); got != before {
		t.Fatalf("disabled model triggered a backend load reloads=%d want=%d", got, before)
	}

	enabled := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: true}, "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%s", enabled.Code, enabled.Body.String())
	}
	chatAgain := httptest.NewRecorder()
	service.ServeHTTP(chatAgain, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"model-a","messages":[]}`)))
	if chatAgain.Code == http.StatusNotFound {
		t.Fatalf("re-enabled model still rejected status=%d body=%s", chatAgain.Code, chatAgain.Body.String())
	}
	if got := backend.reloads.Load(); got == before {
		t.Fatal("re-enabled model did not trigger a backend load")
	}
}

func TestMasterModelStateUsesRegisteredNodeAndRefreshesSnapshot(t *testing.T) {
	var received cluster.ModelStateRequest
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/models/state" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected remote request path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		openai.WriteJSON(w, http.StatusOK, cluster.Snapshot{NodeID: "node-b", Models: []cluster.Model{{LocalID: "remote", Filename: "remote.kcpps", HasLLM: true, Disabled: true}}})
	}))
	defer remote.Close()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master.invalid")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "node-b", NodeURL: remote.URL, Models: []cluster.Model{{LocalID: "remote", Filename: "remote.kcpps", HasLLM: true}}}); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Catalog: catalog.New(t.TempDir()), Registry: registry, ClusterRole: cluster.RoleMaster, NodeID: "master", NodeURL: "http://master.invalid", ClusterToken: "secret", Logger: log.New(io.Discard, "", 0)})
	recorder := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-b", LocalID: "remote", Enabled: false}, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("remote state status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if received.NodeID != "node-b" || received.LocalID != "remote" || received.Enabled {
		t.Fatalf("unexpected forwarded request %#v", received)
	}
	if !registry.ConfigDisabled("node-b", "remote.kcpps") {
		t.Fatal("master registry did not accept refreshed remote snapshot")
	}
	remote.Close()
	unavailable := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-b", LocalID: "remote", Enabled: true}, "")
	if unavailable.Code != http.StatusBadGateway {
		t.Fatalf("unavailable node status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}
}

func TestDisabledModelUnloadsOnlyAfterActiveRuntimeReleaseAndReenableCancels(t *testing.T) {
	service, _, registry := newModelStateTestService(t)
	backend := service.backend.(*fakeBackend)
	state := service.textRuntime.state
	state.mu.Lock()
	state.filename = "model-a.kcpps"
	state.users = 1
	state.mu.Unlock()
	release := releaseActiveConfigOnce(state)
	disabled := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	if _, _, ok := registry.Acquire("model-a", true); ok {
		t.Fatal("new request acquired disabled model")
	}
	if backend.unloads.Load() != 0 {
		t.Fatal("active runtime unloaded before release")
	}
	release()
	waitForUnloadCount(t, backend, 1)

	state.mu.Lock()
	state.filename = "model-a.kcpps"
	state.users = 1
	state.mu.Unlock()
	release = releaseActiveConfigOnce(state)
	postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "")
	enabled := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: true}, "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%s", enabled.Code, enabled.Body.String())
	}
	release()
	time.Sleep(100 * time.Millisecond)
	if backend.unloads.Load() != 1 {
		t.Fatalf("reenable did not cancel pending unload count=%d", backend.unloads.Load())
	}
}

func newModelStateTestService(t *testing.T) (*Service, *modelstate.Store, *cluster.Registry) {
	t.Helper()
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "model-a.kcpps"), []byte(`{"model_param":"model.gguf"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	modelCatalog := catalog.New(configDir)
	models, err := modelCatalog.List()
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleStandalone, "node-a", "http://node-a.invalid")
	if err := registry.UpdateLocal(cluster.LocalModels(models, "node-a", "http://node-a.invalid", cluster.SourceLocal)); err != nil {
		t.Fatal(err)
	}
	store, err := modelstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backendServer.Close)
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Backend: &fakeBackend{url: backendURL, healthy: true}, Catalog: modelCatalog, Registry: registry, ModelStateStore: store, ClusterRole: cluster.RoleStandalone, ClusterToken: "secret", NodeID: "node-a", NodeURL: "http://node-a.invalid", Logger: log.New(io.Discard, "", 0)})
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	return service, store, registry
}

func postModelState(service *Service, path string, request siteapi.ModelStateRequest, token string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(request)
	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	if token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	service.ServeHTTP(recorder, httpRequest)
	return recorder
}

func waitForUnloadCount(t *testing.T, backend *fakeBackend, expected int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for backend.unloads.Load() != expected && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if backend.unloads.Load() != expected {
		t.Fatalf("unload count=%d expected=%d", backend.unloads.Load(), expected)
	}
}
