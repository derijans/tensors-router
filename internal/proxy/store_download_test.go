package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/routerstore"
	"tensors-router/internal/routerstore/routerstoretest"
)

type nodeStoreFixture struct {
	handle    *routerstore.Handle
	analytics *routeranalytics.Store
	errors    *loaderrors.Store
}

func openNodeStoreFixture(t *testing.T, nodeID string) nodeStoreFixture {
	t.Helper()
	handle := routerstoretest.Open(t, routeranalytics.SchemaModule{}, loaderrors.SchemaModule{})
	analyticsStore, err := routeranalytics.NewStore(routeranalytics.StoreConfig{
		NodeID: nodeID, DB: handle.DB(), ReadDB: handle.Reader(), FlushInterval: time.Hour, Logger: log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analyticsStore.Close(context.Background()) })
	errorStore, err := loaderrors.NewStore(loaderrors.StoreConfig{NodeID: nodeID, DB: handle.DB(), ReadDB: handle.Reader()})
	if err != nil {
		t.Fatal(err)
	}
	return nodeStoreFixture{handle: handle, analytics: analyticsStore, errors: errorStore}
}

func serviceOnNodeStore(t *testing.T, role string, nodeID string, store nodeStoreFixture, registry *cluster.Registry, slaveURLs ...string) *Service {
	t.Helper()
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	return NewService(ServiceConfig{
		Backend:        &fakeBackend{url: backendURL, healthy: true},
		Catalog:        catalog.New(t.TempDir()),
		ClusterRole:    role,
		NodeID:         nodeID,
		NodeURL:        "http://" + nodeID,
		Registry:       registry,
		SlaveURLs:      slaveURLs,
		ClusterToken:   "secret",
		ClusterClient:  cluster.NewClient("secret", slaveURLs...),
		AnalyticsStore: store.analytics,
		LoadErrorStore: store.errors,
		RouterStore:    store.handle,
		Logger:         log.New(io.Discard, "", 0),
	})
}

func TestNodeStoreDownloadServesAConsistentCopyIncludingBufferedEvents(t *testing.T) {
	store := openNodeStoreFixture(t, "node-a")
	service := serviceOnNodeStore(t, cluster.RoleStandalone, "node-a", store, nil)
	now := time.Now()
	store.analytics.Record(routeranalytics.Event{ModelID: "llm-a", Section: routeranalytics.SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})
	if err := store.errors.Record(context.Background(), loaderrors.RecordInput{Phase: loaderrors.PhasePreload, Source: "test", Message: "boom"}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, authorizedClusterRequest(http.MethodGet, nodeStoreDownloadPath))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, `attachment; filename="analytics-node-a-`) || !strings.HasSuffix(disposition, `.sqlite"`) {
		t.Fatalf("content disposition = %q", disposition)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("download is missing nosniff")
	}
	downloaded := openDownloadedDatabase(t, recorder.Body.Bytes())
	if got := countRows(t, downloaded, "analytics_events"); got != 1 {
		t.Fatalf("downloaded copy holds %d analytics events, want the buffered one flushed into it", got)
	}
	if got := countRows(t, downloaded, "load_errors"); got != 1 {
		t.Fatalf("downloaded copy holds %d load errors, want 1", got)
	}
	assertNoSnapshotLeftBeside(t, store.handle.Path())
}

func TestNodeStoreDownloadRequiresTheClusterToken(t *testing.T) {
	store := openNodeStoreFixture(t, "node-a")
	service := serviceOnNodeStore(t, cluster.RoleStandalone, "node-a", store, nil)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, nodeStoreDownloadPath, nil))

	if recorder.Code != http.StatusUnauthorized && recorder.Code != http.StatusForbidden {
		t.Fatalf("an unauthenticated peer request got status %d, want it refused", recorder.Code)
	}
}

func TestSiteStoreDownloadRelaysASlavesDatabaseAsIs(t *testing.T) {
	slaveStore := openNodeStoreFixture(t, "slave-a")
	if err := slaveStore.errors.Record(context.Background(), loaderrors.RecordInput{Phase: loaderrors.PhasePreload, Source: "test", Message: "slave failure"}); err != nil {
		t.Fatal(err)
	}
	slave := serviceOnNodeStore(t, cluster.RoleSlave, "slave-a", slaveStore, nil)
	slaveServer := httptest.NewServer(slave)
	t.Cleanup(slaveServer.Close)

	masterStore := openNodeStoreFixture(t, "master")
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "slave-a", NodeURL: slaveServer.URL, ProtocolVersion: cluster.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	master := serviceOnNodeStore(t, cluster.RoleMaster, "master", masterStore, registry, slaveServer.URL)

	recorder := httptest.NewRecorder()
	master.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/store/download?node_id=slave-a", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "analytics-slave-a-") {
		t.Fatalf("relayed download lost the slave's filename: %q", disposition)
	}
	if got := countRows(t, openDownloadedDatabase(t, recorder.Body.Bytes()), "load_errors"); got != 1 {
		t.Fatalf("relayed copy holds %d load errors, want the slave's 1", got)
	}
}

func TestSiteStoreDownloadRefusesAnUnknownNode(t *testing.T) {
	store := openNodeStoreFixture(t, "master")
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	master := serviceOnNodeStore(t, cluster.RoleMaster, "master", store, registry)

	recorder := httptest.NewRecorder()
	master.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/store/download?node_id=http://169.254.169.254", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: a node is chosen by id from the registry, never by address", recorder.Code)
	}
}

func TestSiteLoadErrorsClearEmptiesEveryNodeAndReportsTheOneThatFailed(t *testing.T) {
	slaveStore := openNodeStoreFixture(t, "slave-a")
	for _, message := range []string{"one", "two"} {
		if err := slaveStore.errors.Record(context.Background(), loaderrors.RecordInput{Phase: loaderrors.PhasePreload, Source: "test", Message: message}); err != nil {
			t.Fatal(err)
		}
	}
	slave := serviceOnNodeStore(t, cluster.RoleSlave, "slave-a", slaveStore, nil)
	slaveServer := httptest.NewServer(slave)
	t.Cleanup(slaveServer.Close)
	brokenSlave := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "disk on fire", http.StatusInternalServerError)
	}))
	t.Cleanup(brokenSlave.Close)

	masterStore := openNodeStoreFixture(t, "master")
	if err := masterStore.errors.Record(context.Background(), loaderrors.RecordInput{Phase: loaderrors.PhasePreload, Source: "test", Message: "master failure"}); err != nil {
		t.Fatal(err)
	}
	master := serviceOnNodeStore(t, cluster.RoleMaster, "master", masterStore, nil, slaveServer.URL, brokenSlave.URL)

	recorder := httptest.NewRecorder()
	master.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/router/v1/site/load-errors", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response loadErrorClearResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Cleared != 3 || len(response.ClearedNodes) != 2 {
		t.Fatalf("response %#v, want 3 cleared across master and slave-a", response)
	}
	if len(response.NodeErrors) != 1 {
		t.Fatalf("response %#v, want exactly the broken node reported", response)
	}
	if got := countRows(t, masterStore.handle.Reader(), "load_errors") + countRows(t, slaveStore.handle.Reader(), "load_errors"); got != 0 {
		t.Fatalf("%d load errors survived the clear", got)
	}
}

func TestLoadErrorsClearIsHiddenOnSlaves(t *testing.T) {
	store := openNodeStoreFixture(t, "slave-a")
	slave := serviceOnNodeStore(t, cluster.RoleSlave, "slave-a", store, nil)

	recorder := httptest.NewRecorder()
	slave.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/router/v1/site/load-errors", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: only a standalone node or the master owns the site API", recorder.Code)
	}
}

func authorizedClusterRequest(method string, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer secret")
	return request
}

func openDownloadedDatabase(t *testing.T, content []byte) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "downloaded.sqlite")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertNoSnapshotLeftBeside(t *testing.T, storePath string) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(storePath), ".snapshot-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("snapshot files left behind: %v", leftovers)
	}
}
