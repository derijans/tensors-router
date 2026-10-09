package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestRemoteWebUIProxyUsesSlaveTokenAndRewritesNodeRedirect(t *testing.T) {
	var sawToken atomic.Bool
	var discoveryRequests atomic.Int64
	remote := httptest.NewServer(remoteKoboldLiteNodeHandler(t, &sawToken, &discoveryRequests))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: remote.URL}); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: mustParseURL(t, "http://127.0.0.1:1"), healthy: false},
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		ClusterToken: "secret",
		SlaveURLs:    []string{remote.URL},
		Logger:       log.New(io.Discard, "", 0),
	})
	service.webUI.session.set("kobold-lite", true)

	recorder := serveKoboldLitePanel(service)
	if recorder.Code != http.StatusFound {
		t.Fatalf("unexpected remote status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawToken.Load() {
		t.Fatalf("expected slave authorization token")
	}
	if location := recorder.Header().Get("Location"); location != "/router/webuis/kobold-lite/next?x=1" {
		t.Fatalf("unexpected remote redirect location %q", location)
	}
	recorder = serveKoboldLitePanel(service)
	if recorder.Code != http.StatusFound || discoveryRequests.Load() != 1 {
		t.Fatalf("route snapshot was not reused status=%d discoveries=%d", recorder.Code, discoveryRequests.Load())
	}
	service.webUI.invalidate()
	recorder = serveKoboldLitePanel(service)
	if recorder.Code != http.StatusFound || discoveryRequests.Load() != 2 {
		t.Fatalf("route snapshot invalidation failed status=%d discoveries=%d", recorder.Code, discoveryRequests.Load())
	}
}

func serveKoboldLitePanel(service *Service) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lite/panel?tab=1", nil))
	return recorder
}

func remoteKoboldLiteNodeHandler(t *testing.T, sawToken *atomic.Bool, discoveryRequests *atomic.Int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		sawToken.Store(true)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/webuis":
			discoveryRequests.Add(1)
			writeRemoteActiveWebUI(t, w, remoteURL(r), "slave-a")
		case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/webuis/kobold-lite/panel":
			redirectRemotePanel(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func redirectRemotePanel(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "tab=1" {
		http.Error(w, "bad query", http.StatusBadRequest)
		return
	}
	w.Header().Set("Location", "/router/v1/node/webuis/kobold-lite/next?x=1")
	w.WriteHeader(http.StatusFound)
}
