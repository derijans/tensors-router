package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestUnreachableNodeKeepsItsRegisteredID(t *testing.T) {
	service := newMasterWithUnreachableSlave(t)

	inventory := requestSiteInventory(t, service, "/router/v1/site/inventory")
	if len(inventory.Nodes) != 2 || inventory.Nodes[1].NodeID != "slave" || inventory.Nodes[1].Available {
		t.Fatalf("unreachable node lost its id %#v", inventory.Nodes)
	}

	var flush routeranalytics.FlushResponse
	requestSiteJSON(t, service, http.MethodPost, "/router/v1/site/analytics/flush", &flush)
	if len(flush.NodeErrors) != 1 || flush.NodeErrors[0].NodeID != "slave" {
		t.Fatalf("unreachable node flush error lost its id %#v", flush.NodeErrors)
	}

	var analytics routeranalytics.Response
	requestSiteJSON(t, service, http.MethodGet, "/router/v1/site/analytics", &analytics)
	if len(analytics.NodeErrors) != 1 || analytics.NodeErrors[0].NodeID != "slave" {
		t.Fatalf("unreachable node analytics error lost its id %#v", analytics.NodeErrors)
	}
}

func requestSiteJSON(t *testing.T, service *Service, method string, path string, target any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s status %d body %s", method, path, recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}

func newMasterWithUnreachableSlave(t *testing.T) *Service {
	t.Helper()
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master.invalid")
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave", NodeURL: unreachable.URL, Models: []cluster.Model{}}); err != nil {
		t.Fatal(err)
	}
	return NewService(ServiceConfig{Catalog: catalog.New(t.TempDir()), Registry: registry, ClusterRole: cluster.RoleMaster, NodeID: "master", NodeURL: "http://master.invalid", ClusterToken: "secret", Logger: log.New(io.Discard, "", 0)})
}
