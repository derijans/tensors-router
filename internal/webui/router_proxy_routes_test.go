package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubRoute struct {
	method string
	path   string
	handle http.HandlerFunc
}

func newAuthenticatedRouterStub(t *testing.T, record func(*http.Request), routes ...stubRoute) *httptest.Server {
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if record != nil {
			record(r)
		}
		for _, route := range routes {
			if r.Method == route.method && r.URL.Path == route.path {
				route.handle(w, r)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(router.Close)
	return router
}

func newProxyTestServer(t *testing.T, routerURL string) (*Server, *http.Cookie, string) {
	process := NewRouterProcess(RouterConfig{URL: routerURL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: routerURL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)
	return server, cookie, csrf
}

func serveAdminRequest(server *Server, cookie *http.Cookie, csrf string, method string, path string, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.AddCookie(cookie)
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func requireBodyContaining(fragment string, failure string, respond func(http.ResponseWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), fragment) {
			http.Error(w, failure, http.StatusBadRequest)
			return
		}
		respond(w)
	}
}

func encodeJSONResponse(value any) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func TestServerProxiesNodeStateAndUnloadRoutes(t *testing.T) {
	var seenState, seenUnload, seenInit, seenCancel bool
	router := newAuthenticatedRouterStub(t, nil,
		stubRoute{http.MethodGet, "/router/v1/site/nodes/state", func(w http.ResponseWriter, r *http.Request) {
			seenState = r.URL.Query().Get("node_id") == "node-a"
			writeWebJSON(w, http.StatusOK, map[string]any{"node_id": "node-a", "backends": []any{}, "active_requests": []any{}})
		}},
		stubRoute{http.MethodPost, "/router/v1/site/nodes/unload", func(w http.ResponseWriter, r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			seenUnload = strings.Contains(string(content), `"runtime_id":"kobold-text"`)
			writeWebJSON(w, http.StatusOK, map[string]bool{"ok": true})
		}},
		stubRoute{http.MethodPost, "/router/v1/site/nodes/backends/init", func(w http.ResponseWriter, r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			seenInit = strings.Contains(string(content), `"backend_id":"vllm"`)
			writeWebJSON(w, http.StatusAccepted, map[string]any{"job_id": "job-1", "backend_id": "vllm", "state": "running"})
		}},
		stubRoute{http.MethodPost, "/router/v1/site/nodes/backends/init/cancel", func(w http.ResponseWriter, r *http.Request) {
			seenCancel = true
			writeWebJSON(w, http.StatusOK, map[string]any{"job_id": "job-1", "backend_id": "vllm", "state": "cancelled"})
		}},
	)
	server, cookie, csrf := newProxyTestServer(t, router.URL)

	stateRecorder := serveAdminRequest(server, cookie, "", http.MethodGet, "/api/nodes/state?node_id=node-a", "")
	if stateRecorder.Code != http.StatusOK || !seenState {
		t.Fatalf("state proxy status=%d seen=%t body=%s", stateRecorder.Code, seenState, stateRecorder.Body.String())
	}
	unloadRecorder := serveAdminRequest(server, cookie, csrf, http.MethodPost, "/api/nodes/unload", `{"node_id":"node-a","backend_id":"koboldcpp","runtime_id":"kobold-text","expected_generation":1}`)
	if unloadRecorder.Code != http.StatusOK || !seenUnload {
		t.Fatalf("unload proxy status=%d seen=%t body=%s", unloadRecorder.Code, seenUnload, unloadRecorder.Body.String())
	}
	for _, requestPath := range []string{"/api/nodes/backends/init", "/api/nodes/backends/init/cancel"} {
		recorder := serveAdminRequest(server, cookie, csrf, http.MethodPost, requestPath, `{"node_id":"node-a","backend_id":"vllm"}`)
		if recorder.Code < 200 || recorder.Code >= 300 {
			t.Fatalf("%s status=%d body=%s", requestPath, recorder.Code, recorder.Body.String())
		}
	}
	if !seenInit || !seenCancel {
		t.Fatalf("init proxy=%t cancel proxy=%t", seenInit, seenCancel)
	}
}

func TestServerProxiesBenchmarkRoutes(t *testing.T) {
	seen := []string{}
	router := newAuthenticatedRouterStub(t, func(r *http.Request) { seen = append(seen, r.Method+" "+r.URL.RequestURI()) },
		stubRoute{http.MethodGet, "/router/v1/benchmarks", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("node_id") != "local" || r.URL.Query().Get("model_id") != "model-a" {
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"model_id": "model-a"})
		}},
		stubRoute{http.MethodPost, "/router/v1/benchmarks/run", requireBodyContaining(`"model_id":"model-a"`, "bad body", encodeJSONResponse(map[string]any{"ok": true}))},
	)
	server, cookie, csrf := newProxyTestServer(t, router.URL)

	if recorder := serveAdminRequest(server, cookie, "", http.MethodGet, "/api/benchmarks?node_id=local&model_id=model-a", ""); recorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder := serveAdminRequest(server, cookie, csrf, http.MethodPost, "/api/benchmarks/run", `{"model_id":"model-a","type":"general"}`); recorder.Code != http.StatusOK {
		t.Fatalf("unexpected post status %d body %s", recorder.Code, recorder.Body.String())
	}
	expected := []string{"GET /router/v1/benchmarks?node_id=local&model_id=model-a", "POST /router/v1/benchmarks/run"}
	if strings.Join(seen, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("unexpected proxied requests %#v", seen)
	}
}

func TestServerProxiesWebUIRoutesWithCSRF(t *testing.T) {
	seen := []string{}
	emptyList := encodeJSONResponse(map[string]any{"object": "list", "data": []any{}})
	router := newAuthenticatedRouterStub(t, func(r *http.Request) { seen = append(seen, r.Method+" "+r.URL.Path) },
		stubRoute{http.MethodGet, "/router/v1/site/webuis", func(w http.ResponseWriter, r *http.Request) { emptyList(w) }},
		stubRoute{http.MethodPost, "/router/v1/site/webuis/session", requireBodyContaining(`"enabled":true`, "bad session body", emptyList)},
		stubRoute{http.MethodPost, "/router/v1/site/webuis/load", requireBodyContaining(`"model_id":"text"`, "bad load body", encodeJSONResponse(map[string]any{"ok": true, "url": "https://ui.example.test/"}))},
	)
	server, cookie, csrf := newProxyTestServer(t, router.URL)

	if recorder := serveAdminRequest(server, cookie, "", http.MethodGet, "/api/webuis", ""); recorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder := serveAdminRequest(server, cookie, "", http.MethodPost, "/api/webuis/session", `{"id":"local:kobold-lite","enabled":true}`); recorder.Code != http.StatusForbidden {
		t.Fatalf("missing csrf should be forbidden, got %d", recorder.Code)
	}
	if recorder := serveAdminRequest(server, cookie, csrf, http.MethodPost, "/api/webuis/session", `{"id":"local:kobold-lite","enabled":true}`); recorder.Code != http.StatusOK {
		t.Fatalf("unexpected session status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder := serveAdminRequest(server, cookie, csrf, http.MethodPost, "/api/webuis/load", `{"id":"local:kobold-lite","model_id":"text"}`); recorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", recorder.Code, recorder.Body.String())
	}
	expected := []string{"GET /router/v1/site/webuis", "POST /router/v1/site/webuis/session", "POST /router/v1/site/webuis/load"}
	if strings.Join(seen, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("unexpected proxied requests %#v", seen)
	}
}
