package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestNodeRegistrationConflictReturnsStableCodeWithoutRegistryMutation(t *testing.T) {
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	clusterClient := cluster.NewClient("secret", "http://master")
	service := NewService(ServiceConfig{
		Backend:       &fakeBackend{url: backendURL, healthy: true},
		Catalog:       catalog.New(t.TempDir()),
		Registry:      registry,
		ClusterRole:   cluster.RoleMaster,
		ClusterToken:  "secret",
		NodeID:        "master",
		NodeURL:       "http://master",
		SlaveURLs:     []string{"http://slave-a", "http://slave-b"},
		ClusterClient: clusterClient,
		Logger:        log.New(io.Discard, "", 0),
	})
	register := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/router/v1/node/register", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer secret")
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		return recorder
	}
	if recorder := register(`{"node_id":"slave","node_url":"http://slave-a"}`); recorder.Code != http.StatusOK {
		t.Fatalf("initial registration failed status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := clusterClient.AuthorizedBaseURL("http://slave-a"); err != nil {
		t.Fatalf("accepted node URL was not authorized: %v", err)
	}
	baselineRevision := registry.Revision()
	recorder := register(`{"node_id":"slave","node_url":"http://slave-b"}`)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"type":"cluster_error"`) || !strings.Contains(recorder.Body.String(), `"code":"duplicate_node"`) {
		t.Fatalf("unexpected conflict response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if registry.Revision() != baselineRevision || registry.NodeURLsByID()["slave"] != "http://slave-a" {
		t.Fatalf("conflict mutated registry revision=%d urls=%#v", registry.Revision(), registry.NodeURLsByID())
	}
	if _, err := clusterClient.AuthorizedBaseURL("http://slave-b"); err == nil {
		t.Fatal("rejected node URL was authorized")
	}
}
