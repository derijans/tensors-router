package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadCapabilityIsHiddenWithoutSiblingDownloader(t *testing.T) {
	server := NewServer(Config{}, nil, NewSessionManager("admin-secret"))
	cookie, _ := loginForServerTest(t, server)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/download/capabilities", nil)
	request.AddCookie(cookie)
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"available":false`) {
		t.Fatalf("unexpected absent downloader response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDownloadRoutesProxyOnlyWhenDownloaderIsAvailable(t *testing.T) {
	seen := []string{}
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodPost {
			content, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(content), `"repository":"owner/model"`) {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer router.Close()
	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}, DownloaderAvailable: true}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)
	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/download/capabilities", nil)
	getRequest.AddCookie(cookie)
	server.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected capability status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	postRecorder := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/api/download/plan", strings.NewReader(`{"node_id":"local","repository":"owner/model","files":["model.gguf"]}`))
	postRequest.AddCookie(cookie)
	postRequest.Header.Set("X-CSRF-Token", csrf)
	server.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected plan status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}
	if len(seen) != 2 || seen[0] != "GET /router/v1/site/download/capabilities" || seen[1] != "POST /router/v1/site/download/plan" {
		t.Fatalf("unexpected proxied routes %#v", seen)
	}
}
