package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAdminAPIRejectsKnownOversizedBody(t *testing.T) {
	server := NewServer(Config{}, nil, NewSessionManager("secret"))
	request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("{}"))
	request.ContentLength = maxWebUIControlBodyBytes + 1
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
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

func TestServerProxiesRouterWebUIPathWithSession(t *testing.T) {
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Cookie") != "" {
			http.Error(w, "unexpected cookie", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Accept") != "text/html" {
			http.Error(w, "bad accept", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/router/webuis/kobold-lite/api/save" || r.URL.RawQuery != "tab=1" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "payload" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Location", "/router/webuis/kobold-lite/next?x=1")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("proxied"))
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie := backendSessionCookieForServerTest(t, server, "kobold-lite")
	adminCookie, _ := loginForServerTest(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/webuis/kobold-lite/api/save?tab=1", strings.NewReader("payload"))
	request.AddCookie(cookie)
	request.AddCookie(adminCookie)
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Authorization", "Bearer browser-token")
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "proxied" {
		t.Fatalf("unexpected body %q", recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "/router/webuis/kobold-lite/next?x=1" {
		t.Fatalf("unexpected location %q", location)
	}
}

func TestAdminOriginIssuesSingleUseTicketForBackendOrigin(t *testing.T) {
	routerHit := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerHit = true
		http.NotFound(w, r)
	}))
	defer router.Close()
	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{
		Server: ServerConfig{
			BackendUIBind:      "127.0.0.1:8444",
			BackendUIPublicURL: "https://backend.example:8444",
		},
		Router: RouterConfig{URL: router.URL},
	}, process, NewSessionManager("admin-secret"))
	adminCookie, _ := loginForServerTest(t, server)

	adminRecorder := httptest.NewRecorder()
	adminRequest := httptest.NewRequest(http.MethodGet, "https://admin.example:8443/router/webuis/llama/", nil)
	adminRequest.AddCookie(adminCookie)
	server.AdminHandler().ServeHTTP(adminRecorder, adminRequest)
	if adminRecorder.Code != http.StatusFound {
		t.Fatalf("unexpected admin redirect status=%d body=%s", adminRecorder.Code, adminRecorder.Body.String())
	}
	location, err := url.Parse(adminRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Scheme != "https" || location.Host != "backend.example:8444" || location.Query().Get(backendTicketQueryKey) == "" {
		t.Fatalf("unexpected backend redirect %q", location.String())
	}

	exchangeRecorder := httptest.NewRecorder()
	exchangeRequest := httptest.NewRequest(http.MethodGet, location.String(), nil)
	server.BackendUIHandler().ServeHTTP(exchangeRecorder, exchangeRequest)
	if exchangeRecorder.Code != http.StatusFound || len(exchangeRecorder.Result().Cookies()) != 1 {
		t.Fatalf("ticket exchange failed status=%d cookies=%#v", exchangeRecorder.Code, exchangeRecorder.Result().Cookies())
	}
	replayRecorder := httptest.NewRecorder()
	server.BackendUIHandler().ServeHTTP(replayRecorder, httptest.NewRequest(http.MethodGet, location.String(), nil))
	if replayRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("ticket replay returned %d body=%s", replayRecorder.Code, replayRecorder.Body.String())
	}
	if routerHit {
		t.Fatal("ticket redirects reached the router before backend authorization completed")
	}
}

func TestServerBridgesSdcppBackendPathFromWebUIReferer(t *testing.T) {
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Cookie") != "" {
			http.Error(w, "unexpected cookie", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/router/webuis/sdcpp/sdcpp/v1/capabilities" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie := backendSessionCookieForServerTest(t, server, "sdcpp")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://webui.test/sdcpp/v1/capabilities", nil)
	request.AddCookie(cookie)
	request.Header.Set("Referer", "https://webui.test/router/webuis/sdcpp/")
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("unexpected body %s", recorder.Body.String())
	}
}

func TestServerRequiresSessionForWebUIBackendBridge(t *testing.T) {
	routerHit := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerHit = true
		http.NotFound(w, r)
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://webui.test/sdcpp/v1/capabilities", nil)
	request.Header.Set("Referer", "https://webui.test/router/webuis/sdcpp/")
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if routerHit {
		t.Fatal("router should not be reached without a webui session")
	}
}

func TestServerBridgesWebUIBackendRequestBodyQueryAndHeaders(t *testing.T) {
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Cookie") != "" {
			http.Error(w, "unexpected cookie", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-WebUI-Test") != "1" {
			http.Error(w, "missing forwarded header", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/router/webuis/sdcpp/sdcpp/v1/img_gen" || r.URL.RawQuery != "async=true" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"prompt":"cat"}` {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"job_id": "job-a"})
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie := backendSessionCookieForServerTest(t, server, "sdcpp")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "https://webui.test/sdcpp/v1/img_gen?async=true", strings.NewReader(`{"prompt":"cat"}`))
	request.AddCookie(cookie)
	request.Header.Set("Referer", "https://webui.test/router/webuis/sdcpp/")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer browser-token")
	request.Header.Set("X-WebUI-Test", "1")
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestBackendUIOriginCannotReachManagementAPI(t *testing.T) {
	routerHit := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerHit = true
		http.NotFound(w, r)
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))
	cookie := backendSessionCookieForServerTest(t, server, "kobold-lite")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://webui.test/api/session", nil)
	request.AddCookie(cookie)
	request.Header.Set("Referer", "https://webui.test/router/webuis/kobold-lite/")
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if routerHit {
		t.Fatal("router should not be reached for management api")
	}
}

func TestServerBridgesKoboldAPIPathBeforeManagementAPI(t *testing.T) {
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/router/webuis/kobold-lite/api/v1/generate" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"prompt":"hi"`) {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))
	cookie := backendSessionCookieForServerTest(t, server, "kobold-lite")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "https://webui.test/api/v1/generate", strings.NewReader(`{"prompt":"hi"}`))
	request.AddCookie(cookie)
	request.Header.Set("Referer", "https://webui.test/router/webuis/kobold-lite/")
	request.Header.Set("Content-Type", "application/json")
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestServerRequiresSessionForRouterWebUIPath(t *testing.T) {
	routerHit := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerHit = true
		http.NotFound(w, r)
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL}}, process, NewSessionManager("admin-secret"))

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lite/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if routerHit {
		t.Fatal("router should not be reached without a webui session")
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

func backendSessionCookieForServerTest(t *testing.T, server *Server, kind string) *http.Cookie {
	t.Helper()
	ticket := server.access.Issue(kind)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://webui.test/router/webuis/"+kind+"/?"+backendTicketQueryKey+"="+url.QueryEscape(ticket), nil)
	server.BackendUIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusFound {
		t.Fatalf("backend ticket exchange failed status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == backendSessionCookie {
			return cookie
		}
	}
	t.Fatal("backend ticket exchange did not set a session cookie")
	return nil
}

func TestTrustedLANSkipsSessionAndCSRF(t *testing.T) {
	var loadSeen bool
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/router/v1/load" {
			loadSeen = true
			writeWebJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer router.Close()
	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{
		Security: SecurityConfig{Profile: SecurityProfileTrustedLAN},
		Router:   RouterConfig{URL: router.URL},
	}, process, NewSessionManager(""))
	sessionRecorder := httptest.NewRecorder()
	server.ServeHTTP(sessionRecorder, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("trusted LAN session status %d", sessionRecorder.Code)
	}
	loadRecorder := httptest.NewRecorder()
	server.ServeHTTP(loadRecorder, httptest.NewRequest(http.MethodPost, "/api/load", strings.NewReader(`{"model":"a"}`)))
	if loadRecorder.Code != http.StatusOK || !loadSeen {
		t.Fatalf("trusted LAN load status=%d seen=%t body=%s", loadRecorder.Code, loadSeen, loadRecorder.Body.String())
	}
}
