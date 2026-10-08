package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func TestPreloadModelAcceptsEmbeddingOnlyModel(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"embed"}]}`))
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}), map[string]string{
		"embed": `{"nomodel":true,"embeddingsmodel":"C:\\models\\embed.gguf"}`,
	})

	if err := service.PreloadModel(context.Background(), "embed"); err != nil {
		t.Fatal(err)
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one preload reload, got %d", backend.reloads.Load())
	}
}

func TestEmbeddingsPassThroughWithModelValidation(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"a","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
}

func TestSelectorlessEmbeddingUsesLoadedLocalModel(t *testing.T) {
	service, _, embeddingsBackend := newSeparateEmbeddingTestService(t, false)
	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"separate","input":"load"}`)
	models, err := service.catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	runtimeModels := service.modelsWithRuntimeState(context.Background(), cluster.LocalModelsWithBackendMode(models, service.nodeID, service.nodeURL, cluster.SourceLocal, service.backendMode))
	embeddingsLoaded := false
	for _, model := range runtimeModels {
		embeddingsLoaded = embeddingsLoaded || model.LocalID == "separate" && model.EmbeddingsLoaded
	}
	if !embeddingsLoaded {
		t.Fatalf("embedding runtime load state missing %#v", runtimeModels)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"model":"separate"`) {
		t.Fatalf("unexpected selectorless response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if embeddingsBackend.reloads.Load() != 1 {
		t.Fatalf("loaded embedding model was reloaded %d times", embeddingsBackend.reloads.Load())
	}

	useTinyTransportLimits(service)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"input":"selectorless streaming embedding request"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"model":"separate"`) {
		t.Fatalf("unexpected streaming selectorless response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSelectorlessEmbedPassesThroughWithoutLoadedModel(t *testing.T) {
	var forwardedBody string
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		forwardedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/embed", strings.NewReader(`{"input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || forwardedBody != `{"input":"hello"}` {
		t.Fatalf("unexpected passthrough status=%d body=%q response=%s", recorder.Code, forwardedBody, recorder.Body.String())
	}
}

func TestSelectorlessEmbeddingsRoundRobinLoadedClusterModels(t *testing.T) {
	var hitsMu sync.Mutex
	hits := make([]string, 0, 3)
	newRemote := func(id string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"model":"`+id+`"`) {
				t.Errorf("selected model missing from %s body %s", id, body)
			}
			hitsMu.Lock()
			hits = append(hits, id)
			hitsMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"` + id + `","data":[]}`))
		}))
	}
	remoteA := newRemote("embed-a")
	defer remoteA.Close()
	remoteB := newRemote("embed-b")
	defer remoteB.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	for _, remote := range []struct {
		id     string
		nodeID string
		url    string
	}{
		{id: "embed-a", nodeID: "node-a", url: remoteA.URL},
		{id: "embed-b", nodeID: "node-b", url: remoteB.URL},
	} {
		model := testClusterEmbeddingModel(remote.id, remote.nodeID, remote.id+"-hash", remote.id+"-config", cluster.SourceSlave)
		model.EmbeddingsLoaded = true
		if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: remote.nodeID, NodeURL: remote.url, Models: []cluster.Model{model}}); err != nil {
			t.Fatal(err)
		}
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("selectorless request unexpectedly reached local backend")
	}), "secret")
	for range 3 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"input":"hello"}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
		}
	}

	hitsMu.Lock()
	defer hitsMu.Unlock()
	if len(hits) != 3 || hits[0] != "embed-a" || hits[1] != "embed-b" || hits[2] != "embed-a" {
		t.Fatalf("unexpected round robin order %#v", hits)
	}
}

func TestKoboldEmbedUsesOnlyEmbeddingModels(t *testing.T) {
	var forwardedModel string
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"embed"}]}`))
			return
		}
		if r.URL.Path != "/api/embed" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		forwardedModel, _, err = openai.ModelFromJSON(body)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"embed","data":[]}`))
	}), map[string]string{
		"embed": `{"nomodel":true,"embeddingsmodel":"C:\\models\\embed.gguf"}`,
		"text":  `{"model_param":"C:\\models\\text.gguf"}`,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/embed", strings.NewReader(`{"model":"embed","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || forwardedModel != "embed" || !strings.Contains(recorder.Body.String(), `"model":"embed"`) {
		t.Fatalf("unexpected embed response status=%d model=%q body=%s", recorder.Code, forwardedModel, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/embed", strings.NewReader(`{"model":"text","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected text-only model to be rejected, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestEmbeddingRequestCanUseImageModelAlias(t *testing.T) {
	var sawTextBody bool
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"imageembed"}]}`))
				return
			}
			if r.URL.Path != "/v1/embeddings" {
				t.Fatalf("unexpected text path %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			sawTextBody = strings.Contains(string(body), `"model":"imageembed"`) && !strings.Contains(string(body), "perfectdeliberate")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"imageembed","data":[]}`))
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("unexpected image path %s", r.URL.Path)
		}),
		map[string]string{
			"imageembed": `{"nomodel":true,"sdmodel":"C:\\models\\perfectdeliberate_v90.safetensors","embeddingsmodel":"C:\\models\\embed.gguf"}`,
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"imageembed-perfectdeliberate_v90","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawTextBody {
		t.Fatalf("backend did not receive the text model alias")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"imageembed-perfectdeliberate_v90"`) {
		t.Fatalf("embedding response model was not rewritten: %s", recorder.Body.String())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 0 {
		t.Fatalf("unexpected reload counts text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func TestSeparateEmbeddingRuntimeIsLazyAndIndependentlyUnloadable(t *testing.T) {
	service, textBackend, embeddingsBackend := newSeparateEmbeddingTestService(t, false)

	chat := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"separate","messages":[{"role":"user","content":"hello"}]}`))
	chat.Header.Set("Content-Type", "application/json")
	chatRecorder := httptest.NewRecorder()
	service.ServeHTTP(chatRecorder, chat)
	if chatRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected chat status=%d body=%s", chatRecorder.Code, chatRecorder.Body.String())
	}
	if textBackend.reloads.Load() != 1 || embeddingsBackend.reloads.Load() != 0 {
		t.Fatalf("text request eagerly loaded embeddings text=%d embeddings=%d", textBackend.reloads.Load(), embeddingsBackend.reloads.Load())
	}
	for _, request := range []struct {
		path string
		body string
	}{
		{path: "/v1/images/generations", body: `{"model":"separate-image","prompt":"hello"}`},
		{path: "/v1/audio/speech", body: `{"model":"separate","input":"hello","voice":"default"}`},
		{path: "/api/extra/transcribe", body: `{"model":"separate"}`},
	} {
		httpRequest := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body))
		httpRequest.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httpRequest)
		if recorder.Code != http.StatusOK {
			t.Fatalf("unexpected companion status path=%s status=%d body=%s", request.path, recorder.Code, recorder.Body.String())
		}
		if embeddingsBackend.reloads.Load() != 0 {
			t.Fatalf("companion request eagerly loaded embeddings path=%s", request.path)
		}
	}

	embed := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"separate","input":"hello"}`))
	embed.Header.Set("Content-Type", "application/json")
	embedRecorder := httptest.NewRecorder()
	service.ServeHTTP(embedRecorder, embed)
	if embedRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected embedding status=%d body=%s", embedRecorder.Code, embedRecorder.Body.String())
	}
	if embeddingsBackend.reloads.Load() != 1 || textBackend.reloads.Load() != 1 {
		t.Fatalf("embedding request used wrong runtime text=%d embeddings=%d", textBackend.reloads.Load(), embeddingsBackend.reloads.Load())
	}

	if err := service.unloadLocal(context.Background(), "embeddings"); err != nil {
		t.Fatal(err)
	}
	if embeddingsBackend.unloads.Load() != 1 || textBackend.unloads.Load() != 0 {
		t.Fatalf("embedding unload affected wrong runtime text=%d embeddings=%d", textBackend.unloads.Load(), embeddingsBackend.unloads.Load())
	}
}

func TestStandaloneEmbeddingsCoexistAcrossFamiliesAndSurvivePrimarySwitch(t *testing.T) {
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "kobold-text", `{"backend_mode":"kobold","model_param":"C:/models/text.gguf"}`)
	writeProxyTestConfig(t, dir, "llama-text", `{"backend_mode":"llama_sdcpp","model_param":"C:/models/text.gguf"}`)
	writeProxyTestConfig(t, dir, "kobold-embed", `{"backend_mode":"kobold","nomodel":true,"model":[],"embeddingsmodel":"C:/models/embed.gguf","run_embed_separate":true}`)
	writeProxyTestConfig(t, dir, "llama-embed", `{"backend_mode":"llama_sdcpp","nomodel":true,"model":[],"embeddingsmodel":"C:/models/embed.gguf","run_embed_separate":true}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/extra/version" {
			_, _ = w.Write([]byte(`{"result":"KoboldCpp","embeddings":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
	}))
	t.Cleanup(server.Close)
	backendURL := mustParseURL(t, server.URL)
	koboldText := &fakeBackend{url: backendURL, healthy: true}
	koboldEmbeddings := &fakeBackend{url: backendURL, healthy: true}
	llamaText := &fakeBackend{url: backendURL, healthy: true}
	llamaEmbeddings := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeKobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold:     {TextBackend: koboldText, SeparateBackend: staticSeparateBackend(koboldEmbeddings), StopPrimary: koboldText.Unload},
			BackendModeLlamaSDCPP: {TextBackend: llamaText, SeparateBackend: staticSeparateBackend(llamaEmbeddings), StopPrimary: llamaText.Unload},
		},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})
	postProxyModelRequest(t, service, "/v1/chat/completions", `{"model":"kobold-text","messages":[]}`)
	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"kobold-embed","input":"one"}`)
	postProxyModelRequest(t, service, "/v1/chat/completions", `{"model":"llama-text","messages":[]}`)
	if koboldEmbeddings.unloads.Load() != 0 {
		t.Fatalf("primary switch unloaded the pooled embeddings %d times", koboldEmbeddings.unloads.Load())
	}
	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"llama-embed","input":"two"}`)
	if koboldEmbeddings.unloads.Load() != 0 || llamaEmbeddings.reloads.Load() != 1 {
		t.Fatalf("second family's embeddings did not coexist kobold unloads=%d llama reloads=%d", koboldEmbeddings.unloads.Load(), llamaEmbeddings.reloads.Load())
	}
	if err := service.unloadLocal(context.Background(), "embeddings"); err != nil {
		t.Fatal(err)
	}
	if koboldEmbeddings.unloads.Load() != 1 || llamaEmbeddings.unloads.Load() != 1 {
		t.Fatalf("manual embedding unload missed a pool entry kobold=%d llama=%d", koboldEmbeddings.unloads.Load(), llamaEmbeddings.unloads.Load())
	}
}

func TestPrimaryLoadsPreserveStandaloneEmbeddings(t *testing.T) {
	for _, test := range []struct {
		name        string
		gpu         bool
		wantUnloads int32
	}{
		{name: "CPU coexists", gpu: false, wantUnloads: 0},
		{name: "GPU coexists", gpu: true, wantUnloads: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, embeddingsBackend := newSeparateEmbeddingTestService(t, test.gpu)
			embed := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"separate","input":"hello"}`))
			embed.Header.Set("Content-Type", "application/json")
			embedRecorder := httptest.NewRecorder()
			service.ServeHTTP(embedRecorder, embed)
			if embedRecorder.Code != http.StatusOK {
				t.Fatalf("unexpected embedding status=%d body=%s", embedRecorder.Code, embedRecorder.Body.String())
			}

			chat := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"other","messages":[{"role":"user","content":"hello"}]}`))
			chat.Header.Set("Content-Type", "application/json")
			chatRecorder := httptest.NewRecorder()
			service.ServeHTTP(chatRecorder, chat)
			if chatRecorder.Code != http.StatusOK {
				t.Fatalf("unexpected chat status=%d body=%s", chatRecorder.Code, chatRecorder.Body.String())
			}
			if got := embeddingsBackend.unloads.Load(); got != test.wantUnloads {
				t.Fatalf("unexpected embeddings unloads got=%d want=%d", got, test.wantUnloads)
			}
		})
	}
}

func TestRouterLoadSelectsEmbeddingRuntimeFromCapabilities(t *testing.T) {
	for _, test := range []struct {
		name           string
		config         string
		wantText       int32
		wantEmbeddings int32
		wantImage      int32
	}{
		{
			name: "legacy GPU embedding uses primary runtime",
			config: `{
				"backend_mode":"llama_sdcpp",
				"nomodel":true,
				"model":[],
				"embeddingsmodel":"/models/embed.gguf",
				"embeddingsgpu":true,
				"gpulayers":-1,
				"usecuda":["normal","0"]
			}`,
			wantText: 1,
		},
		{
			name: "separate CPU embedding uses embedding runtime",
			config: `{
				"backend_mode":"llama_sdcpp",
				"nomodel":true,
				"model":[],
				"embeddingsmodel":"/models/embed.gguf",
				"embeddingsgpu":false,
				"run_embed_separate":true,
				"usecuda":["normal","0"]
			}`,
			wantEmbeddings: 1,
		},
		{
			name: "separate GPU embedding uses embedding runtime",
			config: `{
				"backend_mode":"llama_sdcpp",
				"nomodel":true,
				"model":[],
				"embeddingsmodel":"/models/embed.gguf",
				"embeddingsgpu":true,
				"run_embed_separate":true,
				"usecuda":["normal","0"]
			}`,
			wantEmbeddings: 1,
		},
		{
			name: "combined config loads independent runtimes",
			config: `{
				"backend_mode":"llama_sdcpp",
				"model_param":"/models/text.gguf",
				"sdmodel":"/models/image.safetensors",
				"embeddingsmodel":"/models/embed.gguf",
				"embeddingsgpu":true,
				"run_embed_separate":true,
				"usecuda":["normal","0"]
			}`,
			wantText: 1, wantEmbeddings: 1, wantImage: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, textBackend, embeddingsBackend, imageBackend := newExplicitEmbeddingLoadService(t, test.config)
			postRouterLoad(t, service, `{"model":"embed"}`)
			if textBackend.reloads.Load() != test.wantText || embeddingsBackend.reloads.Load() != test.wantEmbeddings || imageBackend.reloads.Load() != test.wantImage {
				t.Fatalf("unexpected reloads text=%d embeddings=%d image=%d", textBackend.reloads.Load(), embeddingsBackend.reloads.Load(), imageBackend.reloads.Load())
			}
		})
	}
}

func TestKoboldRouterLoadSeparateEmbeddingsUsesVersionHealth(t *testing.T) {
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "separate", `{
		"backend_mode":"kobold",
		"model_param":"C:/models/text.gguf",
		"embeddingsmodel":"C:/models/embed.gguf",
		"run_embed_separate":true
	}`)

	var textModelProbes atomic.Int32
	var embeddingVersionProbes atomic.Int32
	var embeddingRequests atomic.Int32
	textServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			textModelProbes.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"text"}]}`))
			return
		}
		t.Fatalf("unexpected text request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(textServer.Close)
	embeddingsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"inactive"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/extra/version":
			ready := embeddingVersionProbes.Add(1) > 1
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"result":"KoboldCpp","embeddings":%t}`, ready)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/embeddings":
			embeddingRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.25]}]}`))
		default:
			t.Fatalf("unexpected embeddings request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(embeddingsServer.Close)

	textBackend := &fakeBackend{url: mustParseURL(t, textServer.URL), healthy: true}
	embeddingsBackend := &fakeBackend{url: mustParseURL(t, embeddingsServer.URL), healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeKobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold: {
				TextBackend:       textBackend,
				EmbeddingsBackend: embeddingsBackend,
			},
		},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})

	postRouterLoad(t, service, `{"model":"separate"}`)
	if textBackend.reloads.Load() != 1 || embeddingsBackend.reloads.Load() != 1 {
		t.Fatalf("unexpected reloads text=%d embeddings=%d", textBackend.reloads.Load(), embeddingsBackend.reloads.Load())
	}
	if textModelProbes.Load() != 1 || embeddingVersionProbes.Load() != 2 {
		t.Fatalf("unexpected readiness probes text=%d embeddings=%d", textModelProbes.Load(), embeddingVersionProbes.Load())
	}

	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"separate","input":"hello"}`)
	if embeddingRequests.Load() != 1 {
		t.Fatalf("embedding request did not reach the embedding backend: %d", embeddingRequests.Load())
	}
}

func newExplicitEmbeddingLoadService(t *testing.T, configContent string) (*Service, *fakeBackend, *fakeBackend, *fakeBackend) {
	t.Helper()
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "embed", configContent)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		case "/sdapi/v1/sd-models":
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
		default:
			t.Fatalf("unexpected readiness path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	backendURL := mustParseURL(t, server.URL)
	textBackend := &fakeBackend{url: backendURL, healthy: true}
	embeddingsBackend := &fakeBackend{url: backendURL, healthy: true}
	imageBackend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeLlamaSDCPP,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeLlamaSDCPP: {
				TextBackend:     textBackend,
				ImageBackend:    imageBackend,
				SeparateBackend: staticSeparateBackend(embeddingsBackend),
			},
		},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})
	return service, textBackend, embeddingsBackend, imageBackend
}

func newSeparateEmbeddingTestService(t *testing.T, gpu bool) (*Service, *fakeBackend, *fakeBackend) {
	t.Helper()
	dir := t.TempDir()
	separateConfig := fmt.Sprintf(`{
		"model_param":"C:/models/text.gguf",
		"sdmodel":"C:/models/image.safetensors",
		"ttsmodel":"C:/models/tts.gguf",
		"whispermodel":"C:/models/whisper.gguf",
		"embeddingsmodel":"C:/models/embed.gguf",
		"embeddingsgpu":%t,
		"run_embed_separate":true
	}`, gpu)
	writeProxyTestConfig(t, dir, "separate", separateConfig)
	writeProxyTestConfig(t, dir, "other", `{"model_param":"C:/models/other.gguf","router_unload_policy":"all"}`)

	backendHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/extra/version" {
			_, _ = w.Write([]byte(`{"tts":true,"transcribe":true,"embeddings":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
	}
	textServer := httptest.NewServer(http.HandlerFunc(backendHandler))
	t.Cleanup(textServer.Close)
	embeddingsServer := httptest.NewServer(http.HandlerFunc(backendHandler))
	t.Cleanup(embeddingsServer.Close)
	textBackend := &fakeBackend{url: mustParseURL(t, textServer.URL), healthy: true}
	embeddingsBackend := &fakeBackend{url: mustParseURL(t, embeddingsServer.URL), healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeKobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold: {
				TextBackend:     textBackend,
				ImageBackend:    textBackend,
				SeparateBackend: staticSeparateBackend(embeddingsBackend),
			},
		},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})
	return service, textBackend, embeddingsBackend
}

func staticSeparateBackend(backend Backend) separateBackendFactory {
	return func(string, string) (Backend, error) { return backend, nil }
}
