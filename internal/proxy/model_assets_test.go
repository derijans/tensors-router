package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"tensors-router/internal/modelassets"
)

func TestEnsureModelAssetsResolvesPortableConfig(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "weights.gguf")
	if err := os.WriteFile(assetPath, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	asset, err := index.IndexFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "portable.kcpps")
	content := `{"model_param_hash":"` + asset.SHA256 + `","model_param_filename":"weights.gguf"}`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Catalog: catalog.New(root), ConfigDir: root, AssetIndex: index})
	if err := service.ensureModelAssets(context.Background(), "portable.kcpps"); err != nil {
		t.Fatal(err)
	}
	resolved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(resolved), "_hash") || !strings.Contains(string(resolved), "weights.gguf") {
		t.Fatalf("portable config was not resolved: %s", resolved)
	}
}

func TestNodeAssetEndpointsRequireTokenAndServeIndexedAsset(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "weights.gguf")
	if err := os.WriteFile(assetPath, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	asset, err := index.IndexFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{ClusterToken: "secret", AssetIndex: index})
	unauthorized := httptest.NewRecorder()
	service.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/router/v1/node/assets/"+asset.SHA256, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected unauthorized status %d", unauthorized.Code)
	}
	streamRequest := httptest.NewRequest(http.MethodGet, "/router/v1/node/assets/"+asset.SHA256, nil)
	streamRequest.Header.Set("Authorization", "Bearer secret")
	stream := httptest.NewRecorder()
	service.ServeHTTP(stream, streamRequest)
	if stream.Code != http.StatusOK || stream.Body.String() != "weights" {
		t.Fatalf("unexpected stream response %d %q", stream.Code, stream.Body.String())
	}
	lookupBody, err := json.Marshal(assetLookupRequest{Hashes: []string{asset.SHA256}})
	if err != nil {
		t.Fatal(err)
	}
	lookupRequest := httptest.NewRequest(http.MethodPost, "/router/v1/node/assets/lookup", bytes.NewReader(lookupBody))
	lookupRequest.Header.Set("Authorization", "Bearer secret")
	lookup := httptest.NewRecorder()
	service.ServeHTTP(lookup, lookupRequest)
	if lookup.Code != http.StatusOK || !strings.Contains(lookup.Body.String(), asset.SHA256) {
		t.Fatalf("unexpected lookup response %d %q", lookup.Code, lookup.Body.String())
	}
}

func TestNodeAssetEndpointRejectsMultipleRanges(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "weights.gguf")
	if err := os.WriteFile(assetPath, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	asset, err := index.IndexFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{ClusterToken: "secret", AssetIndex: index})
	request := httptest.NewRequest(http.MethodGet, "/router/v1/node/assets/"+asset.SHA256, nil)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Range", "bytes=0-1,3-4")
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("unexpected range status %d", response.Code)
	}
}

func TestPeerAssetPromotionResumesVerifiedPartialFile(t *testing.T) {
	root := t.TempDir()
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), filepath.Join(root, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	service := NewService(ServiceConfig{AssetIndex: index})
	content := []byte("complete peer model payload")
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	partial := service.peerPartialPath(hash)
	if err := os.MkdirAll(filepath.Dir(partial), 0o755); err != nil {
		t.Fatal(err)
	}
	offset := int64(9)
	if err := os.WriteFile(partial, content[:offset], 0o600); err != nil {
		t.Fatal(err)
	}
	if !service.promotePeerAsset(bytes.NewReader(content[offset:]), hash, "model.gguf", int64(len(content)), offset, true) {
		t.Fatal("resumed promotion failed")
	}
	path, found := index.Find(hash, "model.gguf")
	if !found {
		t.Fatal("promoted asset was not indexed")
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("unexpected promoted content %q error=%v", actual, err)
	}
}

func TestConcurrentPeerResolutionUsesOneDirectTransfer(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceIndex, err := modelassets.NewIndex(filepath.Join(sourceRoot, "store"), filepath.Join(sourceRoot, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceIndex.Close() })
	sourcePath := filepath.Join(sourceRoot, "model.gguf")
	if err := os.WriteFile(sourcePath, []byte("peer weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset, err := sourceIndex.IndexFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sourceService := NewService(ServiceConfig{ClusterToken: "secret", AssetIndex: sourceIndex})
	var streams atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/router/v1/node/assets/"+asset.SHA256 {
			streams.Add(1)
		}
		sourceService.ServeHTTP(w, r)
	}))
	defer server.Close()

	destinationRoot := t.TempDir()
	destinationIndex, err := modelassets.NewIndex(filepath.Join(destinationRoot, "store"), filepath.Join(destinationRoot, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = destinationIndex.Close() })
	registry := cluster.NewRegistry(cluster.RoleMaster, "destination", "http://destination.invalid")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "source", NodeURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	destination := NewService(ServiceConfig{NodeID: "destination", NodeURL: "http://destination.invalid", ClusterToken: "secret", Registry: registry, ClusterClient: cluster.NewClient("secret", server.URL), AssetIndex: destinationIndex})
	var group sync.WaitGroup
	for request := 0; request < 8; request++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, found := destination.resolvePeerAssetPath(asset.SHA256, asset.Filename); !found {
				t.Errorf("peer resolution failed")
			}
		}()
	}
	group.Wait()
	if streams.Load() != 1 {
		t.Fatalf("expected one peer stream, got %d", streams.Load())
	}
}

func TestInferenceWaitsForDirectPeerResolutionBeforeGeneration(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceIndex, err := modelassets.NewIndex(filepath.Join(sourceRoot, "store"), filepath.Join(sourceRoot, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceIndex.Close() })
	sourcePath := filepath.Join(sourceRoot, "model.gguf")
	if err := os.WriteFile(sourcePath, []byte("peer generation weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset, err := sourceIndex.IndexFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sourceService := NewService(ServiceConfig{ClusterToken: "secret", AssetIndex: sourceIndex})
	sourceServer := httptest.NewServer(sourceService)
	defer sourceServer.Close()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "portable.kcpps")
	if err := os.WriteFile(configPath, []byte(`{"model_param_hash":"`+asset.SHA256+`","model_param_filename":"model.gguf"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	modelCatalog := catalog.New(configDir)
	models, err := modelCatalog.List()
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "destination", "http://destination.invalid")
	if err := registry.UpdateLocal(cluster.LocalModels(models, "destination", "http://destination.invalid", cluster.SourceMaster)); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "source", NodeURL: sourceServer.URL}); err != nil {
		t.Fatal(err)
	}
	var generationRequests atomic.Int32
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" {
			if generationRequests.Add(1) == 1 {
				http.Error(w, "model is not loaded", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","created":1,"model":"backend","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer backendServer.Close()
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	destinationRoot := t.TempDir()
	destinationIndex, err := modelassets.NewIndex(filepath.Join(destinationRoot, "store"), filepath.Join(destinationRoot, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = destinationIndex.Close() })
	var serviceLogs bytes.Buffer
	service := NewService(ServiceConfig{
		Backend:       &fakeBackend{url: backendURL, healthy: true},
		Catalog:       modelCatalog,
		Registry:      registry,
		ClusterRole:   cluster.RoleMaster,
		NodeID:        "destination",
		NodeURL:       "http://destination.invalid",
		ClusterToken:  "secret",
		ClusterClient: cluster.NewClient("secret", sourceServer.URL),
		ConfigDir:     configDir,
		AssetIndex:    destinationIndex,
		Logger:        log.New(&serviceLogs, "", 0),
	})
	service.backendRetryAttempts = 1
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"portable","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"model":"portable"`) {
		t.Fatalf("inference did not complete after resolution status=%d body=%s logs=%s", response.Code, response.Body.String(), serviceLogs.String())
	}
	resolved, err := os.ReadFile(configPath)
	if err != nil || strings.Contains(string(resolved), "_hash") {
		t.Fatalf("config was not resolved before generation content=%s error=%v logs=%s", resolved, err, serviceLogs.String())
	}
}
