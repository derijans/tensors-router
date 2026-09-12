package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	routeranalytics "tensors-router/internal/analytics"
)

func TestPromptBytesRecordsTheRawBodyLength(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"backend\",\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	rawBody := `{"model":"llm","messages":[{"role":"user","content":"hi"}],"stream":true}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(rawBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	response := queryProxyAnalytics(t, service.analyticsStore)
	requestEvent := recentEventOfType(t, response, routeranalytics.EventTypeRequest)
	if requestEvent.PromptBytes != int64(len(rawBody)) {
		t.Fatalf("prompt_bytes = %d, want the raw body length %d", requestEvent.PromptBytes, len(rawBody))
	}
}
