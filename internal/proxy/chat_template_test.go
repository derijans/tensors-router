package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/recipes"
)

func TestChatTemplateVariantsReuseRuntimeAndMergeRequestKwargs(t *testing.T) {
	var mutex sync.Mutex
	received := []map[string]any{}
	service, backend := newTestServiceWithConfigContents(t, http.NotFoundHandler(), map[string]string{
		"think":    `{"model_param":"model.gguf","jinja_kwargs":"{\"enable_thinking\":true,\"mode\":\"configured\"}"}`,
		"no-think": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false,"mode":"configured"}}`,
		"client":   `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false,"mode":"configured"},"router_jinja_kwargs_precedence":"client"}`,
	})
	service.client = chatTemplateTestClient(t, func(request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(content, &value); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		received = append(received, value)
		mutex.Unlock()
	})
	if _, ok := service.client.Transport.(roundTripFunc); !ok {
		t.Fatalf("unexpected test transport %T", service.client.Transport)
	}

	postChatTemplateRequest(t, service, `{"model":"think","messages":[],"chat_template_kwargs":{"mode":"client","client_only":1}}`)
	postChatTemplateRequest(t, service, `{"model":"no-think","messages":[],"chat_template_kwargs":null}`)
	postChatTemplateRequest(t, service, `{"model":"client","messages":[],"chat_template_kwargs":{"enable_thinking":true,"mode":"client"}}`)

	if backend.reloads.Load() != 1 {
		t.Fatalf("compatible variants reloaded %d times", backend.reloads.Load())
	}
	if active := currentRuntimeConfigFilename(service.textRuntime); active != "client.kcpps" {
		t.Fatalf("unexpected logical active config %q", active)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(received) != 3 {
		t.Fatalf("backend received %d requests", len(received))
	}
	assertChatTemplateKwargs(t, received[0], true, "configured", float64(1))
	assertChatTemplateKwargs(t, received[1], false, "configured", nil)
	assertChatTemplateKwargs(t, received[2], true, "client", nil)
}

func TestCompatibleChatTemplateVariantsKeepConcurrentRequestKwargsIsolated(t *testing.T) {
	started := make(chan map[string]any, 2)
	releaseBackend := make(chan struct{})
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request map[string]any
		if err := json.Unmarshal(content, &request); err != nil {
			t.Error(err)
			return
		}
		started <- request
		<-releaseBackend
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ready"}}]}`))
	}), map[string]string{
		"think":    `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true}}`,
		"no-think": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false}}`,
	})

	done := make(chan *httptest.ResponseRecorder, 2)
	for _, body := range []string{
		`{"model":"think","messages":[],"chat_template_kwargs":{"request":"first"}}`,
		`{"model":"no-think","messages":[],"chat_template_kwargs":{"request":"second"}}`,
	} {
		body := body
		go func() {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			service.ServeHTTP(recorder, request)
			done <- recorder
		}()
	}

	requests := []map[string]any{waitChatTemplateRequest(t, started), waitChatTemplateRequest(t, started)}
	close(releaseBackend)
	for range 2 {
		select {
		case recorder := <-done:
			if recorder.Code != http.StatusOK {
				t.Fatalf("unexpected response %d %s", recorder.Code, recorder.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent request did not complete")
		}
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("concurrent compatible variants reloaded %d times", backend.reloads.Load())
	}
	seen := map[bool]string{}
	for _, request := range requests {
		kwargs := request["chat_template_kwargs"].(map[string]any)
		seen[kwargs["enable_thinking"].(bool)] = kwargs["request"].(string)
	}
	if seen[true] != "first" || seen[false] != "second" {
		t.Fatalf("request kwargs leaked across variants: %#v", seen)
	}
}

func TestLlamaSDCPPChatTemplateVariantsReuseRuntime(t *testing.T) {
	var mutex sync.Mutex
	received := []map[string]any{}
	service, textBackend, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"think":    `{"backend_mode":"llama_sdcpp","model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true}}`,
		"no-think": `{"backend_mode":"llama_sdcpp","model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false}}`,
	})
	service.client = chatTemplateTestClient(t, func(request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(content, &value); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		received = append(received, value)
		mutex.Unlock()
	})

	postChatTemplateRequest(t, service, `{"model":"think","messages":[]}`)
	postChatTemplateRequest(t, service, `{"model":"no-think","messages":[]}`)

	if textBackend.reloads.Load() != 1 {
		t.Fatalf("compatible llama.cpp variants reloaded %d times", textBackend.reloads.Load())
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(received) != 2 {
		t.Fatalf("backend received %d requests", len(received))
	}
	assertChatTemplateKwargs(t, received[0], true, "", nil)
	assertChatTemplateKwargs(t, received[1], false, "", nil)
}

func TestRemoteClusterNodeAppliesLocalChatTemplateProfile(t *testing.T) {
	var mutex sync.Mutex
	var received map[string]any
	node, nodeBackend := newTestServiceWithConfigContents(t, http.NotFoundHandler(), map[string]string{
		"remote": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true}}`,
	})
	node.clusterToken = "secret"
	node.client = chatTemplateTestClient(t, func(request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(content, &value); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		received = value
		mutex.Unlock()
	})
	nodeServer := httptest.NewServer(node)
	t.Cleanup(nodeServer.Close)

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "node",
		NodeURL: nodeServer.URL,
		Models:  []cluster.Model{testClusterModel("remote", "node", "model-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}
	master, _ := newTestServiceWithRegistry(t, registry, http.NotFoundHandler(), "secret")

	postChatTemplateRequest(t, master, `{"model":"remote","messages":[],"chat_template_kwargs":{"client":1}}`)

	if nodeBackend.reloads.Load() != 1 {
		t.Fatalf("remote node reloaded %d times", nodeBackend.reloads.Load())
	}
	mutex.Lock()
	defer mutex.Unlock()
	assertChatTemplateKwargs(t, received, true, "", nil)
	kwargs := received["chat_template_kwargs"].(map[string]any)
	if kwargs["client"] != float64(1) {
		t.Fatalf("remote client kwargs were not preserved: %#v", kwargs)
	}
}

func TestLocalRecipeAppliesChatTemplateProfile(t *testing.T) {
	var mutex sync.Mutex
	var received map[string]any
	service, backend := newTestServiceWithConfigContents(t, http.NotFoundHandler(), map[string]string{
		"backend": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true}}`,
	})
	store, err := recipes.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(recipes.Recipe{
		ID:       "recipe",
		PublicID: "recipe",
		Text: &recipes.Component{
			Kind:           recipes.KindText,
			NodeID:         service.nodeID,
			ModelID:        "backend",
			ConfigFilename: "backend.kcpps",
		},
	}, false); err != nil {
		t.Fatal(err)
	}
	service.recipeStore = store
	service.client = chatTemplateTestClient(t, func(request *http.Request) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(content, &value); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		received = value
		mutex.Unlock()
	})

	postChatTemplateRequest(t, service, `{"model":"recipe","messages":[]}`)

	if backend.reloads.Load() != 1 {
		t.Fatalf("recipe backend reloaded %d times", backend.reloads.Load())
	}
	mutex.Lock()
	defer mutex.Unlock()
	assertChatTemplateKwargs(t, received, true, "", nil)
}

func TestStreamingChatTemplateRequestInjectsConfiguredKwargs(t *testing.T) {
	requests := make(chan map[string]any, 1)
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request map[string]any
		if err := json.Unmarshal(content, &request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ready"}}]}`))
	}), map[string]string{
		"think": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true}}`,
	})
	useTinyTransportLimits(service)
	body := `{"messages":[],"prompt":"` + strings.Repeat("x", 256) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tensors-Model", "think")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("streaming request reload count %d", backend.reloads.Load())
	}
	result := waitChatTemplateRequest(t, requests)
	assertChatTemplateKwargs(t, result, true, "", nil)
}

func TestChatTemplateRequestRejectsInvalidOrDuplicatedClientKwargs(t *testing.T) {
	backendCalled := false
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		backendCalled = true
	}), map[string]string{
		"think": `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true}}`,
	})
	for _, body := range []string{
		`{"model":"think","messages":[],"chat_template_kwargs":true}`,
		`{"model":"think","messages":[],"chat_template_kwargs":{},"chat_template_kwargs":{}}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	if backendCalled || backend.reloads.Load() != 0 {
		t.Fatalf("invalid kwargs reached the backend called=%t reloads=%d", backendCalled, backend.reloads.Load())
	}
}

func TestCompatibleChatTemplateVariantsSkipUnloadAndForcedRecoveryReloads(t *testing.T) {
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "think", `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":true},"router_unload_policy":"all"}`)
	writeProxyTestConfig(t, dir, "no-think", `{"model_param":"model.gguf","jinja_kwargs":{"enable_thinking":false},"router_unload_policy":"all"}`)
	backend := newReadyFakeBackend(t, nil)
	service := NewService(ServiceConfig{
		Backend:   backend,
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(io.Discard, "", 0),
	})
	ctx := context.Background()
	if err := service.loadLocalModel(ctx, "think", "think"); err != nil {
		t.Fatal(err)
	}
	if err := service.loadLocalModel(ctx, "no-think", "no-think"); err != nil {
		t.Fatal(err)
	}
	if backend.reloads.Load() != 1 || backend.unloads.Load() != 0 {
		t.Fatalf("compatible unload policy handling reloaded=%d unloaded=%d", backend.reloads.Load(), backend.unloads.Load())
	}
	release, loaded, err := service.acquireModelConfig(service.textRuntime, ctx, "no-think", "no-think.kcpps", readinessText, true)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !loaded || backend.reloads.Load() != 2 || backend.lastReload != "no-think.kcpps" {
		t.Fatalf("forced recovery did not reload requested config loaded=%t reloads=%d config=%q", loaded, backend.reloads.Load(), backend.lastReload)
	}
}

func postChatTemplateRequest(t *testing.T, service *Service, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func waitChatTemplateRequest(t *testing.T, requests <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not receive request")
		return nil
	}
}

func assertChatTemplateKwargs(t *testing.T, request map[string]any, thinking bool, mode string, clientOnly any) {
	t.Helper()
	kwargs, ok := request["chat_template_kwargs"].(map[string]any)
	if !ok {
		t.Fatalf("missing chat template kwargs %#v", request)
	}
	if kwargs["enable_thinking"] != thinking {
		t.Fatalf("unexpected thinking value %#v", kwargs)
	}
	if mode != "" && kwargs["mode"] != mode {
		t.Fatalf("unexpected mode %#v", kwargs)
	}
	if clientOnly == nil {
		if _, exists := kwargs["client_only"]; exists {
			t.Fatalf("unexpected client_only value %#v", kwargs)
		}
		return
	}
	if kwargs["client_only"] != clientOnly {
		t.Fatalf("unexpected client_only value %#v", kwargs)
	}
}

func chatTemplateTestClient(t *testing.T, capture func(*http.Request)) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet && request.URL.Path == "/v1/models" {
			return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[]}`), nil
		}
		if request.Method == http.MethodPost {
			defer request.Body.Close()
			capture(request)
			return testHTTPResponse(http.StatusOK, "application/json", `{"model":"backend","choices":[{"message":{"content":"ready"}}]}`), nil
		}
		t.Fatalf("unexpected backend request %s %s", request.Method, request.URL.Path)
		return nil, nil
	})}
}
