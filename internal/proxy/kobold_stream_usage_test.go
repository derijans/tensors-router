package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	routeranalytics "tensors-router/internal/analytics"
)

const koboldNativeStreamBody = "event: message\ndata: {\"token\": \"blue\", \"finish_reason\": null}\n\nevent: message\ndata: {\"token\": \"\", \"finish_reason\": \"length\"}\n\n"

func koboldNativeStreamBackend(t *testing.T, generationsPerStream int64) http.HandlerFunc {
	t.Helper()
	var totalGenerations atomic.Int64
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/extra/perf":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"last_input_count": 5, "last_token_count": 20, "total_gens": %d}`, totalGenerations.Load())
		case "/api/extra/generate/stream":
			totalGenerations.Add(generationsPerStream)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(koboldNativeStreamBody))
		default:
			t.Fatalf("unexpected backend path %s", r.URL.Path)
		}
	}
}

func serveKoboldNativeStream(t *testing.T, generationsPerStream int64) (*httptest.ResponseRecorder, routeranalytics.Response) {
	t.Helper()
	service, _ := newTestServiceWithConfigContents(t, koboldNativeStreamBackend(t, generationsPerStream), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analytics.store = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/extra/generate/stream", strings.NewReader(`{"prompt":"The sky is","max_length":20}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	return recorder, queryProxyAnalytics(t, service.analytics.store)
}

func TestKoboldNativeStreamRecordsGenerationCountsTheClientNeverSees(t *testing.T) {
	recorder, response := serveKoboldNativeStream(t, 1)

	if recorder.Body.String() != koboldNativeStreamBody {
		t.Fatalf("client stream was altered %q", recorder.Body.String())
	}
	if response.Summary.InputTokens != 5 || response.Summary.OutputTokens != 20 || response.Summary.TotalTokens != 25 {
		t.Fatalf("kobold native stream counts were not recorded %#v", response.Summary)
	}
	event := recentEventOfType(t, response, routeranalytics.EventTypeRequest)
	if event.FinishReason != "length" || event.Aborted {
		t.Fatalf("stream completion was misread %#v", event)
	}
}

func TestKoboldNativeStreamLeavesCountsUnknownWhenAnotherGenerationFinishedAlongside(t *testing.T) {
	recorder, response := serveKoboldNativeStream(t, 2)

	if recorder.Body.String() != koboldNativeStreamBody {
		t.Fatalf("client stream was altered %q", recorder.Body.String())
	}
	if response.Summary.RequestCount != 1 {
		t.Fatalf("kobold native stream was not recorded %#v", response.Summary)
	}
	if response.Summary.InputTokens != 0 || response.Summary.OutputTokens != 0 {
		t.Fatalf("counts of a generation that may not be this request's were attributed %#v", response.Summary)
	}
}
