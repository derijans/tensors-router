package proxy

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestInferenceWaitsForDirectPeerResolutionBeforeGeneration(t *testing.T) {
	sourceIndex := newAssetIndexForTest(t)
	asset := indexAssetForTest(t, sourceIndex, "model.gguf", []byte("peer generation weights"))
	sourceServer := httptest.NewServer(NewService(ServiceConfig{ClusterToken: "secret", AssetIndex: sourceIndex}))
	defer sourceServer.Close()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "portable.kcpps")
	if err := os.WriteFile(configPath, []byte(`{"model_param_hash":"`+asset.SHA256+`","model_param_filename":"model.gguf"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	modelCatalog := catalog.New(configDir)
	registry := destinationRegistryWithPeer(t, modelCatalog, sourceServer.URL)
	var generationRequests atomic.Int32
	backendServer := httptest.NewServer(generationAfterFirstFailureHandler(&generationRequests))
	defer backendServer.Close()

	var serviceLogs bytes.Buffer
	service := NewService(ServiceConfig{
		Backend:       &fakeBackend{url: mustParseURL(t, backendServer.URL), healthy: true},
		Catalog:       modelCatalog,
		Registry:      registry,
		ClusterRole:   cluster.RoleMaster,
		NodeID:        "destination",
		NodeURL:       "http://destination.invalid",
		ClusterToken:  "secret",
		ClusterClient: cluster.NewClient("secret", sourceServer.URL),
		ConfigDir:     configDir,
		AssetIndex:    newAssetIndexForTest(t),
		Logger:        log.New(&serviceLogs, "", 0),
	})
	service.backendRetryAttempts = 1
	response := httptest.NewRecorder()
	service.ServeHTTP(response, modelJSONRequest("/v1/chat/completions", `{"model":"portable","messages":[]}`, ""))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"model":"portable"`) {
		t.Fatalf("inference did not complete after resolution status=%d body=%s logs=%s", response.Code, response.Body.String(), serviceLogs.String())
	}
	resolved, err := os.ReadFile(configPath)
	if err != nil || strings.Contains(string(resolved), "_hash") {
		t.Fatalf("config was not resolved before generation content=%s error=%v logs=%s", resolved, err, serviceLogs.String())
	}
}

func destinationRegistryWithPeer(t *testing.T, modelCatalog *catalog.Catalog, peerURL string) *cluster.Registry {
	models, err := modelCatalog.List()
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "destination", "http://destination.invalid")
	if err := registry.UpdateLocal(cluster.LocalModels(models, "destination", "http://destination.invalid", cluster.SourceMaster)); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "source", NodeURL: peerURL}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func generationAfterFirstFailureHandler(generationRequests *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			if generationRequests.Add(1) == 1 {
				http.Error(w, "model is not loaded", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","created":1,"model":"backend","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		default:
			http.NotFound(w, r)
		}
	}
}
