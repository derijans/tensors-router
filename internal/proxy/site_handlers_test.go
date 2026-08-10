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
	"tensors-router/internal/hardware"
	"tensors-router/internal/inventory"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
	"tensors-router/internal/siteapi"
)

func TestSplitRecipeRoutesTextAndImageToDifferentNodes(t *testing.T) {
	var textSawLocalModel bool
	var imageSawLocalModel bool
	var textSawAuthorization bool
	var imageSawAuthorization bool

	textNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/chat/completions" {
			t.Fatalf("unexpected text path %s", r.URL.Path)
		}
		textSawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		textSawLocalModel = strings.Contains(string(body), `"model":"llm-local"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llm-local","choices":[]}`))
	}))
	defer textNode.Close()

	imageNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/images/generations" {
			t.Fatalf("unexpected image path %s", r.URL.Path)
		}
		imageSawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		imageSawLocalModel = strings.Contains(string(body), `"model":"image-local-dream"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"image-local-dream","data":[]}`))
	}))
	defer imageNode.Close()

	store, err := recipes.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(recipes.Recipe{
		ID:            "mixed",
		PublicID:      "mixed",
		PublicImageID: "mixed-dream",
		Text: &recipes.Component{
			Kind:           recipes.KindText,
			NodeID:         "text-node",
			NodeURL:        textNode.URL,
			ModelID:        "llm-local",
			ConfigFilename: "llm-local.kcpps",
		},
		Image: &recipes.Component{
			Kind:           recipes.KindImage,
			NodeID:         "image-node",
			NodeURL:        imageNode.URL,
			ModelID:        "image-local",
			ImageID:        "image-local-dream",
			ConfigFilename: "image-local.kcpps",
		},
	}, false); err != nil {
		t.Fatal(err)
	}

	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	for nodeID, nodeURL := range map[string]string{"text-node": textNode.URL, "image-node": imageNode.URL} {
		if err := registry.UpdateNode(cluster.Snapshot{NodeID: nodeID, NodeURL: nodeURL}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Registry:     registry,
		ClusterRole:  cluster.RoleMaster,
		Catalog:      catalog.New(t.TempDir()),
		ClusterToken: "secret",
		NodeID:       "master",
		SlaveURLs:    []string{textNode.URL, imageNode.URL},
		RecipeStore:  store,
		Logger:       log.New(io.Discard, "", 0),
	})

	textRecorder := httptest.NewRecorder()
	textRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mixed","messages":[]}`))
	textRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(textRecorder, textRequest)
	if textRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected text status %d body %s", textRecorder.Code, textRecorder.Body.String())
	}
	if !textSawAuthorization || !textSawLocalModel {
		t.Fatalf("text route failed auth=%t model=%t", textSawAuthorization, textSawLocalModel)
	}
	if !strings.Contains(textRecorder.Body.String(), `"model":"mixed"`) {
		t.Fatalf("text response was not rewritten: %s", textRecorder.Body.String())
	}

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"mixed-dream","prompt":"cat"}`))
	imageRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(imageRecorder, imageRequest)
	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected image status %d body %s", imageRecorder.Code, imageRecorder.Body.String())
	}
	if !imageSawAuthorization || !imageSawLocalModel {
		t.Fatalf("image route failed auth=%t model=%t", imageSawAuthorization, imageSawLocalModel)
	}
	if !strings.Contains(imageRecorder.Body.String(), `"model":"mixed-dream"`) {
		t.Fatalf("image response was not rewritten: %s", imageRecorder.Body.String())
	}
}

func TestSiteCookRejectsNodeURLOverride(t *testing.T) {
	var attackerHits atomic.Int64
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer attacker.Close()

	slave := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer slave.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: slave.URL,
	}); err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		NodeURL:      "http://master",
		ClusterToken: "secret",
		Logger:       log.New(io.Discard, "", 0),
	})

	body := `{"id":"mixed","components":[{"kind":"text","node_id":"slave-a","node_url":"` + attacker.URL + `","source":"config","model_id":"llm"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/cook/preview", strings.NewReader(body))
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected rejected node url override, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if attackerHits.Load() != 0 {
		t.Fatalf("attacker target was reached %d times", attackerHits.Load())
	}
}

func TestSiteInventoryHiddenOnSlave(t *testing.T) {
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:     &fakeBackend{url: backendURL, healthy: true},
		Catalog:     catalog.New(t.TempDir()),
		ClusterRole: "slave",
		NodeID:      "slave-a",
		Logger:      log.New(io.Discard, "", 0),
	})
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/inventory", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected hidden site endpoint, got %d", recorder.Code)
	}
}

func TestSiteInventoryScansModelFilesOnlyWhenRequested(t *testing.T) {
	configDir := t.TempDir()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model.gguf"), []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	service := NewService(ServiceConfig{
		Backend:   &fakeBackend{url: backendURL, healthy: true},
		Catalog:   catalog.New(configDir),
		NodeID:    "node-a",
		FileRoots: []string{root},
		Logger:    log.New(&logs, "", 0),
	})

	light := requestSiteInventory(t, service, "/router/v1/site/inventory")
	if len(light.Nodes) != 1 || len(light.Nodes[0].Files) != 0 {
		t.Fatalf("light inventory unexpectedly scanned files: %#v", light.Nodes)
	}
	if strings.Contains(logs.String(), "model file inventory scan started") {
		t.Fatalf("light inventory emitted scan logs: %s", logs.String())
	}

	full := requestSiteInventory(t, service, "/router/v1/site/inventory?include_files=true")
	if len(full.Nodes) != 1 || len(full.Nodes[0].Files) != 1 {
		t.Fatalf("full inventory did not return model files: %#v", full.Nodes)
	}
	if !strings.Contains(logs.String(), "model file inventory scan completed") {
		t.Fatalf("full inventory completion was not logged: %s", logs.String())
	}
}

func TestSiteInventoryIncludesFilesFromRegisteredNodeWithoutModels(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/site/inventory" || r.URL.Query().Get("include_files") != "true" {
			t.Fatalf("unexpected inventory request %s", r.URL.String())
		}
		openai.WriteJSON(w, http.StatusOK, siteapi.NodeInventory{
			NodeID: "files-only", NodeURL: "", Source: cluster.SourceSlave, Role: cluster.RoleSlave, Available: true,
			Models: []cluster.Model{}, Files: []inventory.FileRecord{{NodeID: "files-only", Path: "D:/models/orphan.gguf", Basename: "orphan.gguf"}},
		})
	}))
	defer remote.Close()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master.invalid")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "files-only", NodeURL: remote.URL, Models: []cluster.Model{}}); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Catalog: catalog.New(t.TempDir()), Registry: registry, ClusterRole: cluster.RoleMaster, NodeID: "master", NodeURL: "http://master.invalid", ClusterToken: "secret", Logger: log.New(io.Discard, "", 0)})
	response := requestSiteInventory(t, service, "/router/v1/site/inventory?include_files=true")
	if len(response.Nodes) != 2 || response.Nodes[1].NodeID != "files-only" || len(response.Nodes[1].Files) != 1 {
		t.Fatalf("registered file-only node was not scanned %#v", response.Nodes)
	}
}

func TestModelFileHashIndexesOnlyInventoryFilesAndSurfacesCachedHash(t *testing.T) {
	configDir := t.TempDir()
	root := t.TempDir()
	modelPath := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(t.TempDir(), "outside.gguf")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := modelassets.NewIndex(filepath.Join(t.TempDir(), "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	service := NewService(ServiceConfig{Catalog: catalog.New(configDir), NodeID: "node-a", FileRoots: []string{root}, AssetIndex: index, Logger: log.New(io.Discard, "", 0)})

	response := requestModelFileHash(t, service, "node-a", modelPath)
	if response.NodeID != "node-a" || response.Path != modelPath || len(response.SHA256) != 64 {
		t.Fatalf("unexpected hash response %#v", response)
	}
	full := requestSiteInventory(t, service, "/router/v1/site/inventory?include_files=true")
	if len(full.Nodes[0].Files) != 1 || full.Nodes[0].Files[0].SHA256 != response.SHA256 {
		t.Fatalf("cached hash missing from inventory %#v", full.Nodes[0].Files)
	}
	if err := os.WriteFile(modelPath, []byte("changed model"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := requestSiteInventory(t, service, "/router/v1/site/inventory?include_files=true")
	if changed.Nodes[0].Files[0].SHA256 != "" {
		t.Fatalf("changed file retained stale hash %#v", changed.Nodes[0].Files[0])
	}

	body, err := json.Marshal(siteapi.ModelFileHashRequest{NodeID: "node-a", Path: outsidePath})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/model-files/hash", strings.NewReader(string(body))))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("outside path was accepted status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestModelFileHashRoutesToOwningNode(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/site/model-files/hash" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected remote request path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var request siteapi.ModelFileHashRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		openai.WriteJSON(w, http.StatusOK, siteapi.ModelFileHashResponse{NodeID: "node-b", Path: request.Path, SHA256: hash})
	}))
	defer remote.Close()
	registry := cluster.NewRegistry(cluster.RoleMaster, "node-a", "http://node-a.invalid")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "node-b", NodeURL: remote.URL}); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Catalog:       catalog.New(t.TempDir()),
		ClusterRole:   cluster.RoleMaster,
		NodeID:        "node-a",
		NodeURL:       "http://node-a.invalid",
		Registry:      registry,
		ClusterToken:  "secret",
		ClusterClient: cluster.NewClient("secret", "http://node-a.invalid"),
		Logger:        log.New(io.Discard, "", 0),
	})
	response := requestModelFileHash(t, service, "node-b", "D:/models/model.gguf")
	if response.NodeID != "node-b" || response.SHA256 != hash {
		t.Fatalf("unexpected routed hash response %#v", response)
	}
}

func requestModelFileHash(t *testing.T, service *Service, nodeID string, path string) siteapi.ModelFileHashResponse {
	t.Helper()
	body, err := json.Marshal(siteapi.ModelFileHashRequest{NodeID: nodeID, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/model-files/hash", strings.NewReader(string(body))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected model file hash status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.ModelFileHashResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func requestSiteInventory(t *testing.T, service *Service, path string) siteapi.InventoryResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected inventory status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.InventoryResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestNodeSiteConfigRequiresClusterToken(t *testing.T) {
	dir := packageTempDir(t)
	root := packageTempDir(t)
	textPath := filepath.Join(root, "text.gguf")
	if err := os.WriteFile(textPath, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Catalog:      catalog.New(dir),
		ClusterRole:  "slave",
		NodeID:       "slave-a",
		ConfigDir:    dir,
		FileRoots:    []string{root},
		ClusterToken: "secret",
		Logger:       log.New(io.Discard, "", 0),
	})

	body := `{"id":"made","dry_run":true,"components":[{"kind":"text","source":"file","file_path":"` + filepath.ToSlash(textPath) + `"}]}`
	unauthorized := httptest.NewRecorder()
	service.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/router/v1/node/site/configs", strings.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/node/site/configs", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
	service.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("expected authorized config preview, got %d body %s", authorized.Code, authorized.Body.String())
	}
}

func TestSiteConfigFileApplyAndDelete(t *testing.T) {
	dir := packageTempDir(t)
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:   &fakeBackend{url: backendURL, healthy: true},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		NodeID:    "node-a",
		Logger:    log.New(io.Discard, "", 0),
	})

	body := `{"id":"edited","overwrite":true,"options":{"baseconfig":"base.kcpps","quiet":true}}`
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/config-file/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected config save, got %d body %s", recorder.Code, recorder.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(dir, "edited.kcpps"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"baseconfig": "base.kcpps"`) {
		t.Fatalf("saved config missing body: %s", string(content))
	}

	deleteBody := `{"id":"edited"}`
	deleteRecorder := httptest.NewRecorder()
	service.ServeHTTP(deleteRecorder, httptest.NewRequest(http.MethodDelete, "/router/v1/site/config-file", strings.NewReader(deleteBody)))
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("expected config delete, got %d body %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "edited.kcpps")); !os.IsNotExist(err) {
		t.Fatalf("expected config file removed, got %v", err)
	}
}

func TestSiteConfigFileRejectsPathLikeID(t *testing.T) {
	dir := packageTempDir(t)
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:   &fakeBackend{url: backendURL, healthy: true},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		NodeID:    "node-a",
		Logger:    log.New(io.Discard, "", 0),
	})

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/config-file/apply", strings.NewReader(`{"id":"../bad","options":{}}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected path-like id rejection, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.kcpps")); !os.IsNotExist(err) {
		t.Fatalf("unexpected config file created, got %v", err)
	}
}

func TestSiteCookRejectsKoboldImageEmbeddingsMix(t *testing.T) {
	dir := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{"nomodel":true,"sdmodel":"dream.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "embed.kcpps"), []byte(`{"nomodel":true,"embeddingsmodel":"embed.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend: &fakeBackend{url: backendURL, healthy: true},
		Catalog: catalog.New(dir),
		NodeID:  "node-a",
		Hardware: hardware.NewStatic(hardware.Info{
			MaxThreads: 32,
			GPUBackend: hardware.GPUBackendCPU,
		}),
		Logger: log.New(io.Discard, "", 0),
	})

	body := `{"id":"mixed","components":[{"kind":"image","source":"config","image_id":"image-dream"},{"kind":"embeddings","source":"config","model_id":"embed"}]}`
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/cook/preview", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation failure, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "kobold_image_embeddings_mix") {
		t.Fatalf("missing validation code: %s", recorder.Body.String())
	}
}

func TestSiteCookRejectsThreadBudgetOverflow(t *testing.T) {
	dir := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "text.kcpps"), []byte(`{"model_param":"text.gguf","threads":12}`), 0o644); err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend: &fakeBackend{url: backendURL, healthy: true},
		Catalog: catalog.New(dir),
		NodeID:  "node-a",
		Hardware: hardware.NewStatic(hardware.Info{
			MaxThreads: 4,
			GPUBackend: hardware.GPUBackendCPU,
		}),
		Logger: log.New(io.Discard, "", 0),
	})

	body := `{"id":"too-hot","components":[{"kind":"text","source":"config","model_id":"text"}]}`
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/cook/preview", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation failure, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "thread_budget_exceeded") {
		t.Fatalf("missing validation code: %s", recorder.Body.String())
	}
}

func packageTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "tmp-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	absolute, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}
