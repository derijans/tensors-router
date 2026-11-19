package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
)

func newMetricsTestService(t *testing.T, enabled bool, configs map[string]string) (*Service, *fakeBackend) {
	t.Helper()
	dir := t.TempDir()
	for id, content := range configs {
		writeProxyTestConfig(t, dir, id, content)
	}
	backend := newReadyFakeBackend(t, nil)
	service := NewService(ServiceConfig{Backend: backend, Catalog: catalog.New(dir), ConfigDir: dir, MetricsEnabled: enabled, Logger: log.New(io.Discard, "", 0)})
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	return service, backend
}

func scrape(t *testing.T, service *Service) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/metrics", nil))
	return recorder.Code, recorder.Body.String()
}

func TestMetricsEndpointIsAbsentUnlessEnabled(t *testing.T) {
	service, _ := newMetricsTestService(t, false, map[string]string{"model-a": `{}`})
	if code, _ := scrape(t, service); code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 while metrics are disabled", code)
	}
}

func TestMetricsCountRequestsWithAnalyticsDisabled(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{}`})
	event := routeranalytics.Event{Route: "chat", ModelID: "model-a", StartedAt: time.Now()}
	response := service.analytics.withResponse(testHTTPResponse(http.StatusOK, "application/json", `{}`), event)
	_ = response.Body.Close()
	service.analytics.recordFailure(routeranalytics.Event{Route: "chat", ModelID: "model-a", StartedAt: time.Now()}, http.StatusBadGateway)

	code, body := scrape(t, service)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	for _, want := range []string{
		`tensors_router_requests_total{route="chat",model="model-a",status="200"} 1`,
		`tensors_router_requests_total{route="chat",model="model-a",status="502"} 1`,
		`tensors_router_request_duration_seconds_count{route="chat"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestMetricsCountAStreamOnlyWhenItsBodyCloses(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{}`})
	response := service.analytics.withResponse(testHTTPResponse(http.StatusOK, "text/event-stream", "data: x\n\n"), routeranalytics.Event{Route: "chat", ModelID: "model-a", StartedAt: time.Now()})

	if _, body := scrape(t, service); strings.Contains(body, "tensors_router_requests_total") {
		t.Fatalf("an open stream was counted as finished:\n%s", body)
	}
	_ = response.Body.Close()
	_ = response.Body.Close()
	if _, body := scrape(t, service); !strings.Contains(body, `tensors_router_requests_total{route="chat",model="model-a",status="200"} 1`) {
		t.Fatalf("closed stream not counted exactly once:\n%s", body)
	}
}

func TestMetricsFoldUnknownModelIDsIntoOther(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{}`})
	service.analytics.recordFailure(routeranalytics.Event{Route: "chat", ModelID: "client-chosen-name", StartedAt: time.Now()}, http.StatusNotFound)

	_, body := scrape(t, service)
	if strings.Contains(body, "client-chosen-name") || !strings.Contains(body, `model="other"`) {
		t.Fatalf("a model ID outside the catalog reached a label:\n%s", body)
	}
}

func TestMetricsReportModelLoadsAndLoadedRuntimes(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")

	_, body := scrape(t, service)
	for _, want := range []string{
		`tensors_router_model_load_duration_seconds_count{backend_mode="kobold",result="ok"} 1`,
		`tensors_router_runtime_loaded{runtime="kobold-text",backend_mode="kobold",model="model-a"} 1`,
		`tensors_router_runtime_users{runtime="kobold-text",backend_mode="kobold"} 0`,
		"tensors_router_transport_budget_limit_bytes ",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestMetricsCountIdleUnloads(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{"router_idle_unload_seconds":1}`})
	clock := &idleTestClock{}
	service.idleUnloads.now = clock.now
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")
	clock.advance(time.Hour)
	service.idleUnloads.sweep(context.Background())

	if _, body := scrape(t, service); !strings.Contains(body, `tensors_router_idle_unloads_total{backend_mode="kobold"} 1`) {
		t.Fatalf("idle unload not counted:\n%s", body)
	}
}

func TestMetricsScrapesRaceSafelyWithRequests(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 20 {
				loadAndRelease(t, service, runtime, "model-a")
				service.analytics.recordFailure(routeranalytics.Event{Route: "chat", ModelID: "model-a", StartedAt: time.Now()}, http.StatusTooManyRequests)
			}
		})
		workers.Go(func() {
			for range 20 {
				if code, _ := scrape(t, service); code != http.StatusOK {
					t.Errorf("scrape status=%d", code)
				}
			}
		})
	}
	workers.Wait()
	if _, body := scrape(t, service); !strings.Contains(body, `status="429"} 80`) {
		t.Fatalf("concurrent requests were lost:\n%s", body)
	}
}
