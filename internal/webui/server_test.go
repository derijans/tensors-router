package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerRootServesIndexWithoutRedirect(t *testing.T) {
	server := NewServer(Config{}, nil, NewSessionManager("secret"))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "" {
		t.Fatalf("unexpected redirect to %q", location)
	}
}

func TestServerProxiesBenchmarkRoutes(t *testing.T) {
	seen := []string{}
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/benchmarks":
			if r.URL.Query().Get("node_id") != "local" || r.URL.Query().Get("model_id") != "model-a" {
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"model_id": "model-a"})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/benchmarks/run":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"model_id":"model-a"`) {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/benchmarks?node_id=local&model_id=model-a", nil)
	getRequest.AddCookie(cookie)
	server.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body %s", getRecorder.Code, getRecorder.Body.String())
	}

	postRecorder := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/api/benchmarks/run", strings.NewReader(`{"model_id":"model-a","type":"general"}`))
	postRequest.AddCookie(cookie)
	postRequest.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected post status %d body %s", postRecorder.Code, postRecorder.Body.String())
	}
	if len(seen) != 2 || seen[0] != "GET /router/v1/benchmarks?node_id=local&model_id=model-a" || seen[1] != "POST /router/v1/benchmarks/run" {
		t.Fatalf("unexpected proxied requests %#v", seen)
	}
}

func TestServerProxiesAnalyticsRoute(t *testing.T) {
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/router/v1/site/analytics" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("period") != "7d" || r.URL.Query().Get("section") != "image" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true})
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))
	cookie, _ := loginForServerTest(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/analytics?period=7d&section=image", nil)
	request.AddCookie(cookie)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected analytics status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"enabled":true`) {
		t.Fatalf("unexpected analytics body %s", recorder.Body.String())
	}
}

func TestServerProxiesWebUIRoutesWithCSRF(t *testing.T) {
	seen := []string{}
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		seen = append(seen, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/webuis":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/webuis/session":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"enabled":true`) {
				http.Error(w, "bad session body", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/webuis/load":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"model_id":"text"`) {
				http.Error(w, "bad load body", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": "https://ui.example.test/"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/webuis", nil)
	getRequest.AddCookie(cookie)
	server.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d body %s", getRecorder.Code, getRecorder.Body.String())
	}

	blockedRecorder := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodPost, "/api/webuis/session", strings.NewReader(`{"id":"local:kobold-lite","enabled":true}`))
	blockedRequest.AddCookie(cookie)
	server.ServeHTTP(blockedRecorder, blockedRequest)
	if blockedRecorder.Code != http.StatusForbidden {
		t.Fatalf("missing csrf should be forbidden, got %d", blockedRecorder.Code)
	}

	sessionRecorder := httptest.NewRecorder()
	sessionRequest := httptest.NewRequest(http.MethodPost, "/api/webuis/session", strings.NewReader(`{"id":"local:kobold-lite","enabled":true}`))
	sessionRequest.AddCookie(cookie)
	sessionRequest.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(sessionRecorder, sessionRequest)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected session status %d body %s", sessionRecorder.Code, sessionRecorder.Body.String())
	}

	loadRecorder := httptest.NewRecorder()
	loadRequest := httptest.NewRequest(http.MethodPost, "/api/webuis/load", strings.NewReader(`{"id":"local:kobold-lite","model_id":"text"}`))
	loadRequest.AddCookie(cookie)
	loadRequest.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(loadRecorder, loadRequest)
	if loadRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", loadRecorder.Code, loadRecorder.Body.String())
	}
	if len(seen) != 3 || seen[0] != "GET /router/v1/site/webuis" || seen[1] != "POST /router/v1/site/webuis/session" || seen[2] != "POST /router/v1/site/webuis/load" {
		t.Fatalf("unexpected proxied requests %#v", seen)
	}
}

func TestServerProxiesExternalRouterShutdown(t *testing.T) {
	shutdownSeen := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/router/v1/shutdown":
			shutdownSeen = true
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL, Token: "router-secret"}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/router/shutdown", nil)
	request.AddCookie(cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected shutdown status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !shutdownSeen {
		t.Fatal("external router shutdown was not proxied")
	}
}

func TestServerProxiesLoadWithCSRF(t *testing.T) {
	seen := []string{}
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/router/v1/load" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"config-a"`) {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	blockedRecorder := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodPost, "/api/load", strings.NewReader(`{"model":"config-a"}`))
	blockedRequest.AddCookie(cookie)
	server.ServeHTTP(blockedRecorder, blockedRequest)
	if blockedRecorder.Code != http.StatusForbidden {
		t.Fatalf("missing csrf should be forbidden, got %d", blockedRecorder.Code)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/load", strings.NewReader(`{"model":"config-a"}`))
	request.AddCookie(cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if len(seen) != 1 || seen[0] != "POST /router/v1/load" {
		t.Fatalf("unexpected proxied requests %#v", seen)
	}
}

func TestServerProxiesUnloadWithCSRF(t *testing.T) {
	seen := []string{}
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodPost || r.URL.Path != "/router/v1/unload" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"target":"image"`) {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	blockedRecorder := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodPost, "/api/unload", strings.NewReader(`{"target":"image"}`))
	blockedRequest.AddCookie(cookie)
	server.ServeHTTP(blockedRecorder, blockedRequest)
	if blockedRecorder.Code != http.StatusForbidden {
		t.Fatalf("missing csrf should be forbidden, got %d", blockedRecorder.Code)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/unload", strings.NewReader(`{"target":"image"}`))
	request.AddCookie(cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if len(seen) != 1 || seen[0] != "POST /router/v1/unload" {
		t.Fatalf("unexpected proxied requests %#v", seen)
	}
}

func loginForServerTest(t *testing.T, server *Server) (*http.Cookie, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"token":"admin-secret"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("login failed %d body %s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("unexpected cookies %#v", cookies)
	}
	var body struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CSRF == "" {
		t.Fatal("missing csrf")
	}
	return cookies[0], body.CSRF
}
