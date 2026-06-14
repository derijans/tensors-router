package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestBenchmarkRunRecordsFailedAndSkippedSections(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" {
			http.Error(w, "llm failed", http.StatusTeapot)
			return
		}
		w.WriteHeader(http.StatusOK)
	}), map[string]string{
		"a": `{"model_param":"text.gguf"}`,
	})
	service.benchmarkStore = newBenchmarkStoreForTest(t)
	service.backendRetryAttempts = 1

	body := `{"model_id":"a","type":"section","sections":["llm","image"],"iterations":1,"timeout_seconds":30}`
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/benchmarks/run", strings.NewReader(body)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	var record routerbenchmark.Record
	if err := json.Unmarshal(recorder.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Latest == nil || record.Latest.Status != routerbenchmark.StatusFailed {
		t.Fatalf("unexpected latest %#v", record.Latest)
	}
	if record.Sections[routerbenchmark.SectionLLM].Status != routerbenchmark.StatusFailed {
		t.Fatalf("expected failed llm section %#v", record.Sections)
	}
	if record.Sections[routerbenchmark.SectionImage].Status != routerbenchmark.StatusSkipped {
		t.Fatalf("expected skipped image section %#v", record.Sections)
	}
}

func TestNodeBenchmarksRequireClusterToken(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	service.clusterToken = "secret"
	service.benchmarkStore = newBenchmarkStoreForTest(t)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/node/benchmarks?model_id=a", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", recorder.Code)
	}
}

func TestBenchmarkDataEnrichesRouterModels(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	store := newBenchmarkStoreForTest(t)
	_, err := store.SaveRun("local", "a", routerbenchmark.TypeSection, []routerbenchmark.Summary{{
		RunID:      "run-1",
		Type:       routerbenchmark.TypeSection,
		Section:    routerbenchmark.SectionRuntime,
		Status:     routerbenchmark.StatusSuccess,
		StartedAt:  1,
		FinishedAt: 2,
		DurationMS: 1,
	}}, map[string]json.RawMessage{})
	if err != nil {
		t.Fatal(err)
	}
	service.benchmarkStore = store

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/models", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"benchmark"`) {
		t.Fatalf("missing benchmark data: %s", recorder.Body.String())
	}
}

func TestBenchmarkRunForRemoteNodeUsesRegisteredNodeURL(t *testing.T) {
	var hits atomic.Int64
	var slave *httptest.Server
	slave = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing cluster token")
		}
		switch r.URL.Path {
		case "/router/v1/node/benchmarks/run":
			hits.Add(1)
			writeBenchmarkRecordForTest(w, "slave-a", "remote")
		case "/router/v1/node/models":
			_ = json.NewEncoder(w).Encode(cluster.Snapshot{
				NodeID:  "slave-a",
				NodeURL: slave.URL,
				Models:  []cluster.Model{testClusterModel("remote", "slave-a", "mhash", "chash", cluster.SourceSlave)},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer slave.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: slave.URL,
		Models:  []cluster.Model{testClusterModel("remote", "slave-a", "mhash", "chash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		NodeURL:      "http://master",
		ClusterToken: "secret",
		Logger:       log.New(io.Discard, "", 0),
	})

	body := `{"node_id":"slave-a","model_id":"remote","type":"section","sections":["runtime"]}`
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/benchmarks/run", strings.NewReader(body)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one slave hit, got %d", hits.Load())
	}
}

func TestBenchmarkSectionAllExpandsToKnownSections(t *testing.T) {
	request, err := normalizeBenchmarkRequest(routerbenchmark.RunRequest{
		Type:     routerbenchmark.TypeSection,
		Sections: []string{routerbenchmark.SectionAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	sections := expandBenchmarkSections(request)
	if len(sections) != len(routerbenchmark.OrderedSections) {
		t.Fatalf("unexpected sections %#v", sections)
	}
	for index, section := range routerbenchmark.OrderedSections {
		if sections[index] != section {
			t.Fatalf("unexpected section order %#v", sections)
		}
	}
}

func TestBenchmarkRejectsUnknownSection(t *testing.T) {
	_, err := normalizeBenchmarkRequest(routerbenchmark.RunRequest{
		Type:     routerbenchmark.TypeSection,
		Sections: []string{"runtime", "bad"},
	})
	if err == nil {
		t.Fatal("expected unknown section error")
	}
}

func newBenchmarkStoreForTest(t *testing.T) *routerbenchmark.Store {
	t.Helper()
	store, err := routerbenchmark.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeBenchmarkRecordForTest(w http.ResponseWriter, nodeID string, modelID string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(routerbenchmark.Record{
		NodeID:  nodeID,
		ModelID: modelID,
		Latest: &routerbenchmark.Summary{
			RunID:      "run",
			Type:       routerbenchmark.TypeSection,
			Section:    routerbenchmark.SectionRuntime,
			Status:     routerbenchmark.StatusSuccess,
			StartedAt:  1,
			FinishedAt: 2,
			DurationMS: 1,
		},
	})
}

func TestNodeBenchmarkRunPersistsLocalRecord(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), map[string]string{
		"a": `{"model_param":"text.gguf"}`,
	})
	service.clusterToken = "secret"
	service.benchmarkStore = newBenchmarkStoreForTest(t)

	request := httptest.NewRequest(http.MethodPost, "/router/v1/node/benchmarks/run", strings.NewReader(`{"model_id":"a","type":"section","sections":["runtime"],"timeout_seconds":30}`))
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if _, ok, err := service.benchmarkStore.Record(service.nodeID, "a"); err != nil || !ok {
		t.Fatalf("expected stored record ok=%t err=%v", ok, err)
	}
}
