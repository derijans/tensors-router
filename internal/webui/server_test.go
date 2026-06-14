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
