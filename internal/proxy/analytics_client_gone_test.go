package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	routeranalytics "tensors-router/internal/analytics"
)

func TestRequestWhoseClientLeftIsRecordedAsAbortedNotAsABackendFailure(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	service.analytics.store = newProxyAnalyticsStore(t, "local")
	clientGone, leave := context.WithCancel(context.Background())
	leave()

	request := httptest.NewRequestWithContext(clientGone, http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(httptest.NewRecorder(), request)

	event := recentEventOfType(t, queryProxyAnalytics(t, service.analytics.store), routeranalytics.EventTypeRequest)
	if event.StatusCode != statusClientClosedRequest || !event.Aborted {
		t.Fatalf("event = status %d aborted=%t, want %d aborted: the backend loaded and was never at fault", event.StatusCode, event.Aborted, statusClientClosedRequest)
	}
}

func TestForwardFailureWithTheClientStillWaitingStaysABackendFailure(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	service.analytics.store = newProxyAnalyticsStore(t, "local")
	event := routeranalytics.Event{NodeID: "local", ModelID: "llm", Section: routeranalytics.SectionLLM, EventType: routeranalytics.EventTypeRequest}

	service.analytics.recordForwardFailure(context.Background(), event, errBackendServingNoModel)

	recorded := recentEventOfType(t, queryProxyAnalytics(t, service.analytics.store), routeranalytics.EventTypeRequest)
	if recorded.StatusCode != http.StatusBadGateway || recorded.Aborted {
		t.Fatalf("event = status %d aborted=%t, want 502 not aborted", recorded.StatusCode, recorded.Aborted)
	}
}
