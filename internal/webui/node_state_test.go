package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerProxiesNodeStateAndUnloadRoutes(t *testing.T) {
	seenState := false
	seenUnload := false
	seenInit := false
	seenCancel := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/nodes/state":
			seenState = r.URL.Query().Get("node_id") == "node-a"
			writeWebJSON(w, http.StatusOK, map[string]any{"node_id": "node-a", "backends": []any{}, "active_requests": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/nodes/unload":
			content, _ := io.ReadAll(r.Body)
			seenUnload = strings.Contains(string(content), `"runtime_id":"kobold-text"`)
			writeWebJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/nodes/backends/init":
			content, _ := io.ReadAll(r.Body)
			seenInit = strings.Contains(string(content), `"backend_id":"vllm"`)
			writeWebJSON(w, http.StatusAccepted, map[string]any{"job_id": "job-1", "backend_id": "vllm", "state": "running"})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/nodes/backends/init/cancel":
			seenCancel = true
			writeWebJSON(w, http.StatusOK, map[string]any{"job_id": "job-1", "backend_id": "vllm", "state": "cancelled"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(router.Close)

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	stateRequest := httptest.NewRequest(http.MethodGet, "/api/nodes/state?node_id=node-a", nil)
	stateRequest.AddCookie(cookie)
	stateRecorder := httptest.NewRecorder()
	server.ServeHTTP(stateRecorder, stateRequest)
	if stateRecorder.Code != http.StatusOK || !seenState {
		t.Fatalf("state proxy status=%d seen=%t body=%s", stateRecorder.Code, seenState, stateRecorder.Body.String())
	}

	unloadRequest := httptest.NewRequest(http.MethodPost, "/api/nodes/unload", strings.NewReader(`{"node_id":"node-a","backend_id":"koboldcpp","runtime_id":"kobold-text","expected_generation":1}`))
	unloadRequest.AddCookie(cookie)
	unloadRequest.Header.Set("X-CSRF-Token", csrf)
	unloadRecorder := httptest.NewRecorder()
	server.ServeHTTP(unloadRecorder, unloadRequest)
	if unloadRecorder.Code != http.StatusOK || !seenUnload {
		t.Fatalf("unload proxy status=%d seen=%t body=%s", unloadRecorder.Code, seenUnload, unloadRecorder.Body.String())
	}

	for _, requestPath := range []string{"/api/nodes/backends/init", "/api/nodes/backends/init/cancel"} {
		request := httptest.NewRequest(http.MethodPost, requestPath, strings.NewReader(`{"node_id":"node-a","backend_id":"vllm"}`))
		request.AddCookie(cookie)
		request.Header.Set("X-CSRF-Token", csrf)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code < 200 || recorder.Code >= 300 {
			t.Fatalf("%s status=%d body=%s", requestPath, recorder.Code, recorder.Body.String())
		}
	}
	if !seenInit || !seenCancel {
		t.Fatalf("init proxy=%t cancel proxy=%t", seenInit, seenCancel)
	}
}
