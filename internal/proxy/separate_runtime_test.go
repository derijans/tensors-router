package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/modelstate"
	"tensors-router/internal/siteapi"
	"tensors-router/internal/unloadpolicy"
)

func newSeparatePoolTestService(t *testing.T, limit int, configs map[string]string) (*Service, *fakeBackend, map[string]*fakeBackend) {
	t.Helper()
	dir := t.TempDir()
	for id, content := range configs {
		writeProxyTestConfig(t, dir, id, content)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		case "/api/extra/version":
			_, _ = w.Write([]byte(`{"result":"KoboldCpp","embeddings":true}`))
		default:
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	t.Cleanup(server.Close)
	backendURL := mustParseURL(t, server.URL)
	shared := &fakeBackend{url: backendURL, healthy: true}
	separates := map[string]*fakeBackend{}
	for id := range configs {
		separates[id] = &fakeBackend{url: backendURL, healthy: true}
	}
	factory := func(name string, _ string) (Backend, error) {
		for id, backend := range separates {
			if strings.Contains(name, id) {
				return backend, nil
			}
		}
		return shared, nil
	}
	service := NewService(ServiceConfig{
		BackendMode:          BackendModeKobold,
		SeparateRuntimeLimit: limit,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold: {TextBackend: shared, ImageBackend: shared, SeparateBackend: factory},
		},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})
	return service, shared, separates
}

func embeddingsConfig(policy string) string {
	body := `{"backend_mode":"kobold","nomodel":true,"model":[],"embeddingsmodel":"C:/models/embed.gguf","run_embed_separate":true`
	if policy != "" {
		body += `,"router_unload_policy":` + policy
	}
	return body + "}"
}

func TestSeparatePoolEvictsLeastRecentlyUsedAtCapIncludingDoNotUnload(t *testing.T) {
	service, _, separates := newSeparatePoolTestService(t, 2, map[string]string{
		"pinned":  embeddingsConfig(`"none"`),
		"embed-b": embeddingsConfig(""),
		"embed-c": embeddingsConfig(""),
	})

	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"pinned","input":"a"}`)
	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"embed-b","input":"b"}`)
	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"embed-c","input":"c"}`)

	if separates["pinned"].unloads.Load() != 1 {
		t.Fatalf("least-recently-used do-not-unload entry was not evicted: unloads=%d", separates["pinned"].unloads.Load())
	}
	if separates["embed-b"].unloads.Load() != 0 || separates["embed-c"].unloads.Load() != 0 {
		t.Fatalf("newer entries were evicted b=%d c=%d", separates["embed-b"].unloads.Load(), separates["embed-c"].unloads.Load())
	}
}

func TestSeparatePoolTriggerEvictionPerKind(t *testing.T) {
	for _, test := range []struct {
		name        string
		policy      string
		wantEvicted bool
	}{
		{name: "lane matches", policy: `"text"`, wantEvicted: true},
		{name: "lane does not match", policy: `"image"`, wantEvicted: false},
		{name: "family matches", policy: `"family:kobold"`, wantEvicted: true},
		{name: "config matches", policy: `["config:txt"]`, wantEvicted: true},
		{name: "do not unload", policy: `"none"`, wantEvicted: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, separates := newSeparatePoolTestService(t, 5, map[string]string{
				"sep": embeddingsConfig(test.policy),
				"txt": `{"backend_mode":"kobold","model_param":"C:/models/text.gguf"}`,
			})
			postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"sep","input":"x"}`)
			if separates["sep"].reloads.Load() != 1 {
				t.Fatalf("separate config did not load: reloads=%d", separates["sep"].reloads.Load())
			}
			postProxyModelRequest(t, service, "/v1/chat/completions", `{"model":"txt","messages":[]}`)
			got := separates["sep"].unloads.Load() > 0
			if got != test.wantEvicted {
				t.Fatalf("eviction=%t want=%t", got, test.wantEvicted)
			}
		})
	}
}

func TestSeparatePoolEntrySurvivesSharedRuntimeLoadUnloadCycle(t *testing.T) {
	service, _, separates := newSeparatePoolTestService(t, 5, map[string]string{
		"sep": embeddingsConfig(""),
		"gpu": `{"backend_mode":"kobold","model_param":"C:/models/gpu.gguf","gpulayers":-1,"router_unload_policy":"all"}`,
	})

	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"sep","input":"x"}`)
	postProxyModelRequest(t, service, "/v1/chat/completions", `{"model":"gpu","messages":[]}`)
	if err := service.unloadLocal(context.Background(), "text"); err != nil {
		t.Fatal(err)
	}
	postProxyModelRequest(t, service, "/v1/chat/completions", `{"model":"gpu","messages":[]}`)

	if separates["sep"].reloads.Load() != 1 || separates["sep"].unloads.Load() != 0 {
		t.Fatalf("pooled config did not survive the shared runtime cycle: reloads=%d unloads=%d", separates["sep"].reloads.Load(), separates["sep"].unloads.Load())
	}
}

func TestSeparateEntryTriggeredBy(t *testing.T) {
	lanes := map[string]struct{}{"text": {}}
	ids := map[string]struct{}{"llm": {}, "llm.kcpps": {}}
	if !separateEntryTriggeredBy(unloadpolicy.Selection{"text"}, "kobold", lanes, ids) {
		t.Fatal("lane trigger should match")
	}
	if separateEntryTriggeredBy(unloadpolicy.Selection{"image"}, "kobold", lanes, ids) {
		t.Fatal("non-matching lane should not match")
	}
	if !separateEntryTriggeredBy(unloadpolicy.Selection{"family:kobold"}, "kobold", lanes, ids) {
		t.Fatal("family trigger should match its mode")
	}
	if separateEntryTriggeredBy(unloadpolicy.Selection{"family:llama_sdcpp"}, "kobold", lanes, ids) {
		t.Fatal("family trigger should not match another mode")
	}
	if !separateEntryTriggeredBy(unloadpolicy.Selection{"config:llm"}, "kobold", lanes, ids) {
		t.Fatal("config trigger should match the loading identity")
	}
	if separateEntryTriggeredBy(unloadpolicy.Selection{"none"}, "kobold", lanes, ids) {
		t.Fatal("none never evicts")
	}
	if !separateEntryTriggeredBy(unloadpolicy.Selection{"all"}, "kobold", lanes, ids) {
		t.Fatal("all always evicts")
	}
}

func newSeparateRuntimeClusterPair(t *testing.T) (master *Service, remoteNodeID string, localModelID string) {
	t.Helper()
	remoteNodeID = "slave-a"
	localModelID = "llm"

	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, localModelID+".kcpps"), []byte(`{"model_param":"model.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	slaveCatalog := catalog.New(configDir)
	slaveModels, err := slaveCatalog.List()
	if err != nil {
		t.Fatal(err)
	}
	slaveRegistry := cluster.NewRegistry(cluster.RoleStandalone, remoteNodeID, "http://slave-a.invalid")
	if err := slaveRegistry.UpdateLocal(cluster.LocalModels(slaveModels, remoteNodeID, "http://slave-a.invalid", cluster.SourceLocal)); err != nil {
		t.Fatal(err)
	}
	store, err := modelstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	slaveBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(slaveBackend.Close)
	slaveBackendURL := mustParseURL(t, slaveBackend.URL)
	slaveService := NewService(ServiceConfig{
		Backend:         &fakeBackend{url: slaveBackendURL, healthy: true},
		Catalog:         slaveCatalog,
		ConfigDir:       configDir,
		Registry:        slaveRegistry,
		ModelStateStore: store,
		ClusterRole:     cluster.RoleStandalone,
		ClusterToken:    "secret",
		NodeID:          remoteNodeID,
		NodeURL:         "http://slave-a.invalid",
		Logger:          log.New(io.Discard, "", 0),
	})
	t.Cleanup(func() { _ = slaveService.Close(context.Background()) })
	slave := httptest.NewServer(http.HandlerFunc(slaveService.ServeHTTP))
	t.Cleanup(slave.Close)

	masterRegistry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master.invalid")
	if err := masterRegistry.UpdateNode(cluster.Snapshot{NodeID: remoteNodeID, NodeURL: slave.URL}); err != nil {
		t.Fatal(err)
	}
	master = NewService(ServiceConfig{
		Catalog:      catalog.New(t.TempDir()),
		Registry:     masterRegistry,
		ClusterRole:  cluster.RoleMaster,
		ClusterToken: "secret",
		NodeID:       "master",
		NodeURL:      "http://master.invalid",
		Logger:       log.New(io.Discard, "", 0),
	})
	return master, remoteNodeID, localModelID
}

func TestSiteSeparateRuntimesStatusFetchReachesRemoteNode(t *testing.T) {
	master, remoteNodeID, localModelID := newSeparateRuntimeClusterPair(t)

	recorder := httptest.NewRecorder()
	target := "/router/v1/site/separate-runtimes?node_id=" + remoteNodeID + "&local_id=" + localModelID
	master.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("remote status fetch failed with %d: %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.SeparateRuntimeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.NodeID != remoteNodeID || response.LocalID != localModelID {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestSiteSeparateRuntimesUpdateReachesRemoteNode(t *testing.T) {
	master, remoteNodeID, localModelID := newSeparateRuntimeClusterPair(t)

	requestBody, err := json.Marshal(siteapi.SeparateRuntimeRequest{
		NodeID:  remoteNodeID,
		LocalID: localModelID,
		Settings: siteapi.SeparateRuntimeSettings{
			RunSeparate: true,
			Triggers:    []string{"none"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/separate-runtimes", strings.NewReader(string(requestBody)))
	master.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("remote update failed with %d: %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.SeparateRuntimeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.HasOverride || !response.Settings.RunSeparate {
		t.Fatalf("update did not persist on the remote node: %#v", response)
	}
}
