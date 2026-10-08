package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestClusterModelsEndpointHidesNodeIdentityAndIndexesConflicts(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models:  []cluster.Model{testClusterModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"id":"same"`) || !strings.Contains(recorder.Body.String(), `"id":"same-2"`) {
		t.Fatalf("missing public models: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "slave-a") {
		t.Fatalf("node identity leaked: %s", recorder.Body.String())
	}
}

func TestClusterFirstLocalRequestLoadsWhenBackendStartsUnhealthy(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("a", "master", "model-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}

	service, backend := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
		default:
			t.Fatalf("unexpected backend path %s", r.URL.Path)
		}
	}), "secret")
	backend.healthy = false

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected startup request to load one model, got %d reloads", backend.reloads.Load())
	}
}

func TestClusterLocalRequestQueuesDuringModelLoad(t *testing.T) {
	for _, requestedModel := range []string{"a", "b"} {
		t.Run(requestedModel, func(t *testing.T) {
			service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
			}), map[string]string{"a": `{}`, "b": `{}`})
			models, err := service.catalog.List()
			if err != nil {
				t.Fatal(err)
			}
			registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
			if err := registry.UpdateLocal(cluster.LocalModels(models, "master", "http://master", cluster.SourceMaster)); err != nil {
				t.Fatal(err)
			}
			service.registry = registry
			backend.healthy = false

			loadStarted := make(chan struct{})
			continueLoad := make(chan struct{})
			var loadStartedOnce sync.Once
			var continueLoadOnce sync.Once
			t.Cleanup(func() {
				continueLoadOnce.Do(func() { close(continueLoad) })
			})
			backend.onReload = func(filename string) {
				if filename != "a.kcpps" {
					return
				}
				loadStartedOnce.Do(func() { close(loadStarted) })
				<-continueLoad
			}

			loadDone := make(chan error, 1)
			go func() {
				loadDone <- service.loadLocalModel(context.Background(), "a", "a")
			}()
			select {
			case <-loadStarted:
			case <-time.After(time.Second):
				t.Fatal("initial model load did not start")
			}

			type requestResult struct {
				status int
				body   string
			}
			requestDone := make(chan requestResult, 1)
			go func() {
				recorder := httptest.NewRecorder()
				body := fmt.Sprintf(`{"model":%q,"messages":[]}`, requestedModel)
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				service.ServeHTTP(recorder, request)
				requestDone <- requestResult{status: recorder.Code, body: recorder.Body.String()}
			}()

			select {
			case result := <-requestDone:
				continueLoadOnce.Do(func() { close(continueLoad) })
				t.Fatalf("request returned during model load with status %d body %s", result.status, result.body)
			case <-time.After(50 * time.Millisecond):
			}
			continueLoadOnce.Do(func() { close(continueLoad) })

			if err := <-loadDone; err != nil {
				t.Fatalf("initial model load failed: %v", err)
			}
			select {
			case result := <-requestDone:
				if result.status != http.StatusOK {
					t.Fatalf("queued request status %d body %s", result.status, result.body)
				}
			case <-time.After(time.Second):
				t.Fatal("queued request did not finish")
			}

			expectedReloads := int32(1)
			if requestedModel == "b" {
				expectedReloads = 2
			}
			if backend.reloads.Load() != expectedReloads {
				t.Fatalf("expected %d reloads, got %d", expectedReloads, backend.reloads.Load())
			}
		})
	}
}

func TestClusterRemoteRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/chat/completions" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"same"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"same-2","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"same-2"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestClusterRemoteTextAPIRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/api/chat" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"same"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","done":true}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"model":"same-2","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"same-2"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestClusterImageModelListUsesPublicImageIDs(t *testing.T) {
	registry := newConflictingImageRegistry(t, "http://slave-a")
	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model_name":"same-dream"`) || !strings.Contains(recorder.Body.String(), `"model_name":"same-2-dream"`) {
		t.Fatalf("missing public image models: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "slave-a") {
		t.Fatalf("node identity leaked: %s", recorder.Body.String())
	}
}

func TestClusterRemoteImageRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalImageModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/images/generations" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalImageModel = strings.Contains(string(body), `"model":"same-dream"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"same-dream","data":[]}`))
	}))
	defer remote.Close()

	registry := newConflictingImageRegistry(t, remote.URL)
	service, backend := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"same-2-dream","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalImageModel {
		t.Fatalf("remote did not receive local image model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"same-2-dream"`) {
		t.Fatalf("image response model was not rewritten: %s", recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("local backend should not reload for remote image request, got %d", backend.reloads.Load())
	}
}

func TestClusterStreamedSdcppJobPollReturnsToSubmittingNode(t *testing.T) {
	var pollSeen atomic.Bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/router/v1/node/inference/sdcpp/v1/vid_gen":
			_, _ = w.Write([]byte(`{"job_id":"remote-job"}`))
		case "/router/v1/node/inference/sdcpp/v1/jobs/remote-job":
			pollSeen.Store(true)
			_, _ = w.Write([]byte(`{"id":"remote-job","status":"done"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	registry := newConflictingImageRegistry(t, remote.URL)
	service, backend := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	useTinyTransportLimits(service)
	submit := httptest.NewRequest(http.MethodPost, "/sdcpp/v1/vid_gen", strings.NewReader(`{"model":"same-2-dream","prompt":"video"}`))
	submit.Header.Set("Content-Type", "application/json")
	submit.Header.Set("X-Tensors-Model", "same-2-dream")
	submitRecorder := httptest.NewRecorder()
	service.ServeHTTP(submitRecorder, submit)
	if submitRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected submit status %d body=%s", submitRecorder.Code, submitRecorder.Body.String())
	}

	pollRecorder := httptest.NewRecorder()
	service.ServeHTTP(pollRecorder, httptest.NewRequest(http.MethodGet, "/sdcpp/v1/jobs/remote-job", nil))
	if pollRecorder.Code != http.StatusOK || !pollSeen.Load() {
		t.Fatalf("remote job poll status=%d seen=%t body=%s", pollRecorder.Code, pollSeen.Load(), pollRecorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("remote job unexpectedly loaded the local backend %d times", backend.reloads.Load())
	}
}

func TestClusterRemoteImageRequestRewritesSelectors(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		body           string
		contentType    string
		headerModel    string
		queryKey       string
		expectedBody   string
		expectedQuery  string
		expectedHeader string
	}{
		{
			name:         "sd checkpoint body",
			path:         "/sdapi/v1/txt2img",
			body:         `{"sd_model_checkpoint":"same-2-dream","prompt":"cat"}`,
			contentType:  "application/json",
			expectedBody: `"sd_model_checkpoint":"same-dream"`,
		},
		{
			name:         "override checkpoint body",
			path:         "/sdapi/v1/txt2img",
			body:         `{"override_settings":{"sd_model_checkpoint":"same-2-dream"},"prompt":"cat"}`,
			contentType:  "application/json",
			expectedBody: `"sd_model_checkpoint":"same-dream"`,
		},
		{
			name:          "model query",
			path:          "/v1/images/generations?model=same-2-dream",
			body:          `{"prompt":"cat"}`,
			contentType:   "application/json",
			queryKey:      "model",
			expectedQuery: "same-dream",
		},
		{
			name:          "sd checkpoint query",
			path:          "/sdapi/v1/txt2img?sd_model_checkpoint=same-2-dream",
			body:          `{"prompt":"cat"}`,
			contentType:   "application/json",
			queryKey:      "sd_model_checkpoint",
			expectedQuery: "same-dream",
		},
		{
			name:           "model header",
			path:           "/sdapi/v1/txt2img",
			body:           `{"prompt":"cat"}`,
			contentType:    "application/json",
			headerModel:    "same-2-dream",
			expectedHeader: "same-dream",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if testCase.expectedBody != "" && !strings.Contains(string(body), testCase.expectedBody) {
					t.Fatalf("remote body missing rewritten selector: %s", string(body))
				}
				if testCase.queryKey != "" && r.URL.Query().Get(testCase.queryKey) != testCase.expectedQuery {
					t.Fatalf("remote query was not rewritten: %s", r.URL.RawQuery)
				}
				if testCase.expectedHeader != "" && r.Header.Get("X-Tensors-Model") != testCase.expectedHeader {
					t.Fatalf("remote header was not rewritten: %s", r.Header.Get("X-Tensors-Model"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"model":"same-dream","data":[]}`))
			}))
			defer remote.Close()

			registry := newConflictingImageRegistry(t, remote.URL)
			service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			if testCase.headerModel != "" {
				request.Header.Set("X-Tensors-Model", testCase.headerModel)
			}
			service.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"model":"same-2-dream"`) {
				t.Fatalf("image response model was not rewritten: %s", recorder.Body.String())
			}
		})
	}
}

func TestClusterInactiveLocalCombinedImageModelIsRejected(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})
	models, err := service.catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal(cluster.LocalModels(models, "master", "http://master", cluster.SourceMaster)); err != nil {
		t.Fatal(err)
	}
	service.registry = registry

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-vision","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestClusterSplitCombinedImageModelIsVisibleWhenInactive(t *testing.T) {
	model := testClusterModel("combo", "master", "model-hash", "config-hash", cluster.SourceMaster)
	model.HasImage = true
	model.ImageID = "combo-dream"
	model.PublicImageID = "combo-dream"
	model.BackendMode = BackendModeLlamaSDCPP
	model.Capabilities.Image = &catalog.ImageCapabilities{Model: "C:/models/dream.safetensors"}

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{model}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	service.backendMode = BackendModeLlamaSDCPP
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model_name":"combo-dream"`) {
		t.Fatalf("combined split image model was hidden: %s", recorder.Body.String())
	}
}

func TestClusterRemoteEmbeddingsRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/embeddings" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"embed"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"embed","data":[]}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterEmbeddingModel("embed", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterEmbeddingModel("embed", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed-2","input":"hello"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local embedding model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"embed-2"`) {
		t.Fatalf("embedding response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestClusterRemoteEmbeddingsRequestCanUsePublicImageID(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/embeddings" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"imageembed"`) && !strings.Contains(string(body), "perfectdeliberate")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"imageembed","data":[]}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterImageEmbeddingModel("imageembed", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave, "perfectdeliberate_v90")},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"imageembed-perfectdeliberate_v90","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local embedding model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"imageembed-perfectdeliberate_v90"`) {
		t.Fatalf("embedding response model was not rewritten: %s", recorder.Body.String())
	}
}

func testClusterModel(id string, nodeID string, modelHash string, configHash string, source string) cluster.Model {
	return cluster.Model{
		PublicID:   id,
		LocalID:    id,
		Filename:   id + ".kcpps",
		Created:    1,
		HasLLM:     true,
		ModelHash:  modelHash,
		ConfigHash: configHash,
		Source:     source,
		NodeID:     nodeID,
		NodeURL:    "http://" + nodeID,
		Available:  true,
	}
}

func testClusterImageModel(id string, nodeID string, modelHash string, configHash string, source string, imageName string) cluster.Model {
	model := testClusterModel(id, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasImage = true
	model.ImageID = id + "-" + imageName
	model.PublicImageID = model.ImageID
	model.Capabilities.Image = &catalog.ImageCapabilities{
		Model: "C:/models/" + imageName + ".safetensors",
	}
	return model
}

func testClusterEmbeddingModel(id string, nodeID string, modelHash string, configHash string, source string) cluster.Model {
	model := testClusterModel(id, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasEmbeddings = true
	model.Capabilities.Embeddings = &catalog.EmbeddingCapability{
		Model: "C:/models/" + id + ".gguf",
	}
	return model
}

func testClusterImageEmbeddingModel(id string, nodeID string, modelHash string, configHash string, source string, imageName string) cluster.Model {
	model := testClusterEmbeddingModel(id, nodeID, modelHash, configHash, source)
	model.HasImage = true
	model.ImageID = id + "-" + imageName
	model.PublicImageID = model.ImageID
	model.BackendMode = BackendModeLlamaSDCPP
	model.Capabilities.Image = &catalog.ImageCapabilities{
		Model: "C:/models/" + imageName + ".safetensors",
	}
	return model
}

func newConflictingImageRegistry(t *testing.T, slaveURL string) *cluster.Registry {
	t.Helper()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterImageModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster, "dream")}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: slaveURL,
		Models:  []cluster.Model{testClusterImageModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave, "dream")},
	}); err != nil {
		t.Fatal(err)
	}
	return registry
}
