package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"tensors-router/internal/cluster"
)

func TestBufferedForwardDoesNotFollowBackendRedirect(t *testing.T) {
	var redirectTargetHits atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"elsewhere","choices":[]}`))
	}))
	t.Cleanup(redirectTarget.Close)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/v1/chat/completions", http.StatusTemporaryRedirect)
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	if hits := redirectTargetHits.Load(); hits != 0 {
		t.Fatalf("backend redirect was followed: target received %d requests, response %d %s", hits, recorder.Code, recorder.Body.String())
	}
}

func TestStreamingOllamaChatRestoresPublicModelInEveryRecord(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"model\":\"backend-local\",\"message\":{\"content\":\"a\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"backend-local\",\"message\":{\"content\":\"\"},\"done\":true}\n"))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)

	request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"model":"llm","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tensors-Model", "llm")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	records := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
	if len(records) != 2 {
		t.Fatalf("expected two NDJSON records, got %d: %s", len(records), recorder.Body.String())
	}
	for _, record := range records {
		if !strings.Contains(record, `"model":"llm"`) || strings.Contains(record, "backend-local") {
			t.Fatalf("NDJSON record leaked the backend model id: %s", record)
		}
	}
}

func TestStreamingBackendServerErrorKeepsBackendMessage(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"context window exceeded"}`))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Tensors-Model", "llm")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadGateway || !strings.Contains(body, "backend_error") || !strings.Contains(body, "context window exceeded") {
		t.Fatalf("backend failure did not match the buffered path: status=%d body=%s", recorder.Code, body)
	}
}

func TestClusterLocalRouteReportsBackendFailure(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("a", "master", "model-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"out of memory"}`))
	}), "secret")
	service.backendRetryDelay = 0

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadGateway || !strings.Contains(body, "backend_error") || !strings.Contains(body, "out of memory") {
		t.Fatalf("local cluster route hid the backend failure: status=%d body=%s", recorder.Code, body)
	}
}
