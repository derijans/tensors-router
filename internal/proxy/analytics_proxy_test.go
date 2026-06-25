package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestAnalyticsRecordsTextRequest(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20},"tokens_per_second":4}`))
	}), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.RequestCount != 1 || response.Summary.TotalTokens != 20 {
		t.Fatalf("unexpected analytics summary %#v", response.Summary)
	}
	if len(response.Recent) != 1 || response.Recent[0].ModelID != "llm" || response.Recent[0].Section != routeranalytics.SectionLLM {
		t.Fatalf("unexpected recent analytics %#v", response.Recent)
	}
}

func TestAnalyticsRecordsStreamingUsage(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"backend\",\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.TotalTokens != 5 || response.Summary.OutputTokens != 3 {
		t.Fatalf("stream usage was not recorded %#v", response.Summary)
	}
}

func TestAnalyticsRecordsImageMetadata(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{},{}]}`))
	}), map[string]string{
		"combo": `{"nomodel":true,"sdmodel":"dream.safetensors"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sdapi/v1/txt2img", strings.NewReader(`{"sd_model_checkpoint":"combo-dream","width":640,"height":384,"batch_size":2}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.ImageCount != 2 {
		t.Fatalf("unexpected image count %#v", response.Summary)
	}
	if len(response.Recent) != 1 || response.Recent[0].ImageWidth != 640 || response.Recent[0].ImageType != "txt2img" {
		t.Fatalf("unexpected image metadata %#v", response.Recent)
	}
}

func TestAnalyticsMasterDoesNotRecordRemoteRoute(t *testing.T) {
	var sawRemote bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRemote = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llm","usage":{"total_tokens":7}}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterModel("llm", "slave-a", "hash", "config", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}
	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("local backend should not receive remote route")
	}), "secret")
	service.analyticsStore = newProxyAnalyticsStore(t, "master")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !sawRemote {
		t.Fatalf("remote route failed status=%d sawRemote=%t body=%s", recorder.Code, sawRemote, recorder.Body.String())
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.RequestCount != 0 {
		t.Fatalf("master recorded remote route %#v", response.Summary)
	}
}

func TestSiteAnalyticsAggregatesRemoteNodes(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing cluster authorization")
		}
		if r.URL.Path != "/router/v1/node/analytics" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(routeranalytics.Response{
			Enabled: true,
			Summary: routeranalytics.Summary{
				RequestCount: 2,
				SuccessCount: 2,
				TotalTokens:  9,
			},
			Nodes: []routeranalytics.NodeUsage{{NodeID: "slave-a", RequestCount: 2, TotalTokens: 9}},
		})
	}))
	defer remote.Close()

	backendURL, err := url.Parse("http://local-backend")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		Backend:       &fakeBackend{url: backendURL, healthy: true},
		Catalog:       catalog.New(t.TempDir()),
		ClusterRole:   cluster.RoleMaster,
		NodeID:        "master",
		NodeURL:       "http://master",
		SlaveURLs:     []string{remote.URL},
		ClusterToken:  "secret",
		ClusterClient: cluster.NewClient("secret", remote.URL),
		Logger:        log.New(io.Discard, "", 0),
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "master")
	now := time.Now()
	service.analyticsStore.Record(routeranalytics.Event{
		ModelID:     "local",
		Section:     routeranalytics.SectionLLM,
		StatusCode:  200,
		Success:     true,
		StartedAt:   now,
		FinishedAt:  now,
		TotalTokens: 5,
	})

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/analytics?period=all", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response routeranalytics.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 3 || response.Summary.TotalTokens != 14 {
		t.Fatalf("unexpected aggregate response %#v", response.Summary)
	}
}

func newProxyAnalyticsStore(t *testing.T, nodeID string) *routeranalytics.Store {
	t.Helper()
	store, err := routeranalytics.NewStore(routeranalytics.StoreConfig{
		NodeID:        nodeID,
		DatabasePath:  filepath.Join(t.TempDir(), "analytics.sqlite"),
		FlushInterval: time.Hour,
		Logger:        log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	return store
}

func queryProxyAnalytics(t *testing.T, store *routeranalytics.Store) routeranalytics.Response {
	t.Helper()
	response, err := store.Query(context.Background(), routeranalytics.Query{Period: routeranalytics.PeriodAll})
	if err != nil {
		t.Fatal(err)
	}
	return response
}
