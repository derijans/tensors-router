package proxy

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	routeranalytics "tensors-router/internal/analytics"
)

func TestAnalyticsRecordsKoboldNativeGenerate(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/generate" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"text":"hi","finish_reason":"stop","prompt_tokens":11,"completion_tokens":153}]}`))
	}), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"prompt":"hi","max_length":16}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.RequestCount != 1 {
		t.Fatalf("kobold native generate was not recorded %#v", response.Summary)
	}
	if response.Summary.InputTokens != 11 || response.Summary.OutputTokens != 153 || response.Summary.TotalTokens != 164 {
		t.Fatalf("unexpected analytics summary %#v", response.Summary)
	}
	event := recentEventOfType(t, response, routeranalytics.EventTypeRequest)
	if event.Section != routeranalytics.SectionLLM {
		t.Fatalf("unexpected recent analytics %#v", event)
	}
}

func TestAnalyticsSkipsOllamaDiscoveryPaths(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tags", nil))

	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.RequestCount != 0 || len(response.Recent) != 0 {
		t.Fatalf("discovery request must not be recorded %#v", response.Recent)
	}
}

func TestAnalyticsRecordsOllamaStreamingCounts(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`{"model":"backend","response":"hi","done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"model":"backend","response":"","done":true,"done_reason":"stop","prompt_eval_count":11,"prompt_eval_duration":1,"eval_count":195,"eval_duration":1}` + "\n"))
	}), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(`{"model":"llm","prompt":"hi","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "&#34;") {
		t.Fatalf("ollama stream reached the client html-escaped %s", recorder.Body.String())
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.InputTokens != 11 || response.Summary.OutputTokens != 195 {
		t.Fatalf("ollama streaming counts were not recorded %#v", response.Summary)
	}
	if response.Summary.AverageDuration <= 0 {
		if response.Summary.AverageTokensPS != 0 {
			t.Fatalf("speed cannot be derived without a measured duration %#v", response.Summary)
		}
		return
	}
	wallClockSpeed := float64(response.Summary.OutputTokens) / (response.Summary.AverageDuration / 1000)
	if math.Abs(response.Summary.AverageTokensPS-wallClockSpeed) > 0.001 {
		t.Fatalf("placeholder eval_duration leaked into the speed %#v", response.Summary)
	}
}
