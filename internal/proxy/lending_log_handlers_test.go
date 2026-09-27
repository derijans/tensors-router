package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/routerstore/routerstoretest"
)

func withDecisionStore(t *testing.T, service *Service) *offloaddecisions.Store {
	t.Helper()
	handle := routerstoretest.Open(t, offloaddecisions.SchemaModule{})
	store, err := offloaddecisions.NewStore(offloaddecisions.StoreConfig{NodeID: service.nodeID, DB: handle.DB(), ReadDB: handle.Reader(), FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service.offloadDecisions = store
	service.scheduler.decisions = store
	return store
}

func newLoggingCluster(t *testing.T) (*Service, *offloaddecisions.Store, *offloaddecisions.Store) {
	t.Helper()
	slave, _ := newTestService(t, http.NotFoundHandler())
	slaveServer := httptest.NewServer(slave)
	t.Cleanup(slaveServer.Close)
	master, _ := newTestService(t, http.NotFoundHandler())
	masterServer := httptest.NewServer(master)
	t.Cleanup(masterServer.Close)
	joinCluster(t, master, "master", cluster.RoleMaster, masterServer.URL, "", slaveServer.URL)
	joinCluster(t, slave, "slave", cluster.RoleSlave, slaveServer.URL, masterServer.URL, masterServer.URL)
	master.registry = cluster.NewRegistry(cluster.RoleMaster, "master", masterServer.URL)
	if err := master.registry.UpdateNode(cluster.Snapshot{NodeID: "slave", NodeURL: slaveServer.URL, ProtocolVersion: cluster.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	return master, withDecisionStore(t, master), withDecisionStore(t, slave)
}

func TestSiteLendingDecisionsMergeEveryNodesLogNewestFirst(t *testing.T) {
	master, masterStore, slaveStore := newLoggingCluster(t)
	now := time.Now()
	masterStore.Record(offloaddecisions.Record{Kind: offloaddecisions.KindPlan, Lane: "image", Outcome: offloaddecisions.OutcomeProbe, RecordedAt: now.Add(-2 * time.Second)})
	slaveStore.Record(offloaddecisions.Record{Kind: offloaddecisions.KindDispatch, Lane: "image", Outcome: offloaddecisions.OutcomeLent, RecordedAt: now.Add(-time.Second)})
	for _, store := range []*offloaddecisions.Store{masterStore, slaveStore} {
		if err := store.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	master.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/offload/decisions?lane=image", nil))
	var response lendingDecisionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s err=%v", recorder.Code, recorder.Body.String(), err)
	}
	if len(response.Records) != 2 || response.Records[0].NodeID != "slave" || response.Records[1].NodeID != "master" || len(response.NodeErrors) != 0 {
		t.Fatalf("records = %+v errors = %+v, want the slave's lent row first, then the master's probe", response.Records, response.NodeErrors)
	}
}

func TestSiteLendingSummaryReportsEveryNodesSettingsFingerprint(t *testing.T) {
	master, _, _ := newLoggingCluster(t)

	recorder := httptest.NewRecorder()
	master.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/offload/summary", nil))
	var response lendingSummaryResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s err=%v", recorder.Code, recorder.Body.String(), err)
	}
	if len(response.Nodes) != 2 || response.Nodes[0].NodeID != "master" || response.Nodes[1].NodeID != "slave" {
		t.Fatalf("nodes = %+v, want master and slave", response.Nodes)
	}
	if response.Nodes[0].SettingsFingerprint == "" || response.Nodes[0].SettingsFingerprint != response.Nodes[1].SettingsFingerprint {
		t.Fatalf("fingerprints %q and %q, want both nodes on the same defaults", response.Nodes[0].SettingsFingerprint, response.Nodes[1].SettingsFingerprint)
	}
	if response.Leases == nil || response.Nodes[1].HeldRequests == nil {
		t.Fatalf("summary %+v, want empty lists rather than null", response)
	}
}
