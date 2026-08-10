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
}
