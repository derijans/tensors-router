package webui

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestServerMapsEveryAPIRouteToItsRouterTarget(t *testing.T) {
	var mutex sync.Mutex
	var seen string
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		seen = r.Method + " " + r.URL.Path
		mutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer router.Close()

	process := NewRouterProcess(RouterConfig{URL: router.URL}, t.TempDir())
	server := NewServer(Config{Router: RouterConfig{URL: router.URL, Token: "router-secret"}}, process, NewSessionManager("admin-secret"))
	cookie, csrf := loginForServerTest(t, server)

	for _, testCase := range []struct {
		method   string
		path     string
		upstream string
	}{
		{http.MethodGet, "/api/inventory", "GET /router/v1/site/inventory"},
		{http.MethodGet, "/api/nodes/state", "GET /router/v1/site/nodes/state"},
		{http.MethodPost, "/api/nodes/unload", "POST /router/v1/site/nodes/unload"},
		{http.MethodPost, "/api/nodes/backends/init", "POST /router/v1/site/nodes/backends/init"},
		{http.MethodPost, "/api/nodes/backends/init/cancel", "POST /router/v1/site/nodes/backends/init/cancel"},
		{http.MethodGet, "/api/nodes/backends/launch-options", "GET /router/v1/site/nodes/backends/launch-options"},
		{http.MethodPost, "/api/nodes/backends/launch-options", "POST /router/v1/site/nodes/backends/launch-options"},
		{http.MethodPost, "/api/models/state", "POST /router/v1/site/models/state"},
		{http.MethodGet, "/api/routing-groups", "GET /router/v1/site/routing-groups"},
		{http.MethodPost, "/api/routing-groups", "POST /router/v1/site/routing-groups"},
		{http.MethodDelete, "/api/routing-groups", "DELETE /router/v1/site/routing-groups"},
		{http.MethodGet, "/api/text-routing-groups", "GET /router/v1/site/text-routing-groups"},
		{http.MethodPost, "/api/text-routing-groups", "POST /router/v1/site/text-routing-groups"},
		{http.MethodDelete, "/api/text-routing-groups", "DELETE /router/v1/site/text-routing-groups"},
		{http.MethodGet, "/api/separate-runtimes", "GET /router/v1/site/separate-runtimes"},
		{http.MethodPost, "/api/separate-runtimes", "POST /router/v1/site/separate-runtimes"},
		{http.MethodGet, "/api/download/capabilities", "GET /router/v1/site/download/capabilities"},
		{http.MethodPost, "/api/download/search", "POST /router/v1/site/download/search"},
		{http.MethodPost, "/api/download/search-page", "POST /router/v1/site/download/search-page"},
		{http.MethodPost, "/api/download/repository", "POST /router/v1/site/download/repository"},
		{http.MethodPost, "/api/download/plan", "POST /router/v1/site/download/plan"},
		{http.MethodPost, "/api/download/jobs", "POST /router/v1/site/download/jobs"},
		{http.MethodGet, "/api/download/jobs/job-1", "GET /router/v1/site/download/jobs/job-1"},
		{http.MethodPost, "/api/download/jobs/job-1/cancel", "POST /router/v1/site/download/jobs/job-1/cancel"},
		{http.MethodGet, "/api/download/library", "GET /router/v1/site/download/library"},
		{http.MethodPost, "/api/download/rescan", "POST /router/v1/site/download/rescan"},
		{http.MethodGet, "/api/webuis", "GET /router/v1/site/webuis"},
		{http.MethodPost, "/api/webuis/session", "POST /router/v1/site/webuis/session"},
		{http.MethodPost, "/api/webuis/load", "POST /router/v1/site/webuis/load"},
		{http.MethodGet, "/api/benchmarks", "GET /router/v1/benchmarks"},
		{http.MethodPost, "/api/benchmarks/run", "POST /router/v1/benchmarks/run"},
		{http.MethodGet, "/api/analytics", "GET /router/v1/site/analytics"},
		{http.MethodPost, "/api/analytics/flush", "POST /router/v1/site/analytics/flush"},
		{http.MethodGet, "/api/load-captures", "GET /router/v1/site/load-captures"},
		{http.MethodGet, "/api/load-captures/capture-1", "GET /router/v1/site/load-captures/capture-1"},
		{http.MethodGet, "/api/load-errors", "GET /router/v1/site/load-errors"},
		{http.MethodGet, "/api/offload/settings", "GET /router/v1/site/offload/settings"},
		{http.MethodPost, "/api/offload/settings", "POST /router/v1/site/offload/settings"},
		{http.MethodDelete, "/api/offload/settings", "DELETE /router/v1/site/offload/settings"},
		{http.MethodGet, "/api/offload/decisions", "GET /router/v1/site/offload/decisions"},
		{http.MethodGet, "/api/offload/summary", "GET /router/v1/site/offload/summary"},
		{http.MethodPost, "/api/load", "POST /router/v1/load"},
		{http.MethodPost, "/api/cook/preview", "POST /router/v1/site/cook/preview"},
		{http.MethodPost, "/api/cook/apply", "POST /router/v1/site/cook/apply"},
		{http.MethodDelete, "/api/cook/recipe-1", "DELETE /router/v1/site/cook/recipe-1"},
		{http.MethodPost, "/api/config-file/preview", "POST /router/v1/site/config-file/preview"},
		{http.MethodPost, "/api/config-file/apply", "POST /router/v1/site/config-file/apply"},
		{http.MethodDelete, "/api/config-file", "DELETE /router/v1/site/config-file"},
		{http.MethodPost, "/api/model-assets/export", "POST /router/v1/site/model-assets/export"},
		{http.MethodPost, "/api/model-files/hash", "POST /router/v1/site/model-files/hash"},
		{http.MethodPost, "/api/model-assets/resolve", "POST /router/v1/site/model-assets/resolve"},
		{http.MethodPost, "/api/model-assets/resolve-batch", "POST /router/v1/site/model-assets/resolve-batch"},
		{http.MethodPost, "/api/model-assets/jobs", "POST /router/v1/site/model-assets/jobs"},
		{http.MethodPost, "/api/model-assets/bind", "POST /router/v1/site/model-assets/bind"},
		{http.MethodPost, "/api/model-assets/candidates", "POST /router/v1/site/model-assets/candidates"},
		{http.MethodPost, "/api/model-assets/substitute", "POST /router/v1/site/model-assets/substitute"},
		{http.MethodGet, "/api/model-assets/jobs/job-1", "GET /router/v1/site/model-assets/jobs/job-1"},
		{http.MethodGet, "/api/model-assets/asset-1", "GET /router/v1/site/model-assets/asset-1"},
		{http.MethodGet, "/api/model-assets/jobs", "GET /router/v1/site/model-assets/jobs"},
		{http.MethodPost, "/api/inventory", ""},
		{http.MethodGet, "/api/load", ""},
		{http.MethodPut, "/api/routing-groups", ""},
		{http.MethodGet, "/api/cook/recipe-1", ""},
		{http.MethodPost, "/api/model-assets/asset-1", ""},
		{http.MethodGet, "/api/unknown", ""},
	} {
		t.Run(testCase.method+" "+testCase.path, func(t *testing.T) {
			mutex.Lock()
			seen = ""
			mutex.Unlock()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			request.AddCookie(cookie)
			request.Header.Set("X-CSRF-Token", csrf)
			server.ServeHTTP(recorder, request)
			mutex.Lock()
			upstream := seen
			mutex.Unlock()
			if upstream != testCase.upstream {
				t.Fatalf("upstream %q, want %q (status %d)", upstream, testCase.upstream, recorder.Code)
			}
			if testCase.upstream == "" && recorder.Code != http.StatusNotFound {
				t.Fatalf("unmatched route status %d, want 404", recorder.Code)
			}
		})
	}
}
