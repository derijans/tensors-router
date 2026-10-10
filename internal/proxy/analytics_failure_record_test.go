package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
)

func TestBackendFailureIsRecordedWithItsUpstreamStatusAndReason(t *testing.T) {
	const upstreamReason = "request exceeds the available context size"
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
			return
		}
		http.Error(w, upstreamReason, http.StatusInternalServerError)
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	service.backendRetryDelay = time.Millisecond
	service.backendRetryMaxDelay = time.Millisecond
	service.analytics.store = newProxyAnalyticsStore(t, "local")

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("client status = %d, want %d: retry and mapping behaviour is not part of this change", recorder.Code, http.StatusBadGateway)
	}
	event := recentEventOfType(t, queryProxyAnalytics(t, service.analytics.store), routeranalytics.EventTypeRequest)
	if event.UpstreamStatus != http.StatusInternalServerError {
		t.Fatalf("upstream status = %d, want %d", event.UpstreamStatus, http.StatusInternalServerError)
	}
	if !strings.Contains(event.ErrorMessage, upstreamReason) {
		t.Fatalf("error message %q does not carry the backend's reason", event.ErrorMessage)
	}
}

func TestRecordedErrorMessageIsBoundedOnAValidRuneBoundary(t *testing.T) {
	store := newProxyAnalyticsStore(t, "local")
	store.Record(routeranalytics.Event{
		NodeID:       "local",
		Section:      routeranalytics.SectionLLM,
		ErrorMessage: strings.Repeat("ž", 600),
	})

	event := recentEventOfType(t, queryProxyAnalytics(t, store), routeranalytics.EventTypeRequest)
	if len(event.ErrorMessage) == 0 || len(event.ErrorMessage) > 512 {
		t.Fatalf("stored error message is %d bytes, want 1..512", len(event.ErrorMessage))
	}
	if strings.ContainsRune(event.ErrorMessage, '�') {
		t.Fatal("the cap split a multi-byte rune")
	}
}

func TestQueueWaitIsTheTimeBetweenArrivalAndTheStartOfBackendWork(t *testing.T) {
	store := newProxyAnalyticsStore(t, "local")
	arrived := time.Now().Add(-10 * time.Second)
	store.Record(routeranalytics.Event{
		NodeID:        "local",
		Section:       routeranalytics.SectionLLM,
		StartedAt:     arrived,
		FinishedAt:    arrived.Add(10 * time.Second),
		WorkStartedAt: arrived.Add(4 * time.Second),
		StatusCode:    http.StatusOK,
		Success:       true,
	})

	event := recentEventOfType(t, queryProxyAnalytics(t, store), routeranalytics.EventTypeRequest)
	if event.QueueWaitMS != 4000 {
		t.Fatalf("queue wait = %d ms, want 4000", event.QueueWaitMS)
	}
}
