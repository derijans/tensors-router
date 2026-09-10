package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const koboldUsageChunk = `data: {"id":"c","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":13,"completion_tokens":11,"total_tokens":24}}`

func koboldStreamBackend(t *testing.T, seen *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		*seen = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hi"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"finish_reason":"stop","delta":{}}]}` + "\n\n"))
		if requestAsksForStreamUsage(*seen) {
			_, _ = w.Write([]byte(koboldUsageChunk + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}
}

func requestAsksForStreamUsage(body string) bool {
	var request struct {
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		return false
	}
	return request.StreamOptions.IncludeUsage
}

func TestStreamUsageInjectedForKoboldAndStrippedFromClient(t *testing.T) {
	var seen string
	service, _ := newTestServiceWithConfigContents(t, koboldStreamBackend(t, &seen), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if !strings.Contains(seen, `"stream_options":{"include_usage":true}`) {
		t.Fatalf("include_usage was not injected, backend saw %s", seen)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "usage") {
		t.Fatalf("injected usage chunk reached the client %s", body)
	}
	if !strings.Contains(body, "[DONE]") || !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("stripping damaged the client stream %s", body)
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.InputTokens != 13 || response.Summary.OutputTokens != 11 {
		t.Fatalf("injected usage did not reach analytics %#v", response.Summary)
	}
}

func TestClientRequestedUsagePassesThroughUntouched(t *testing.T) {
	var seen string
	service, _ := newTestServiceWithConfigContents(t, koboldStreamBackend(t, &seen), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analyticsStore = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true,"stream_options":{"include_usage":true}}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if strings.Count(seen, "include_usage") != 1 {
		t.Fatalf("client stream_options was rewritten, backend saw %s", seen)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"usage"`) {
		t.Fatalf("client asked for usage and must still receive it %s", body)
	}
	if !strings.Contains(body, `"total_tokens":24`) {
		t.Fatalf("client usage chunk was altered %s", body)
	}
	response := queryProxyAnalytics(t, service.analyticsStore)
	if response.Summary.OutputTokens != 11 {
		t.Fatalf("client-requested usage did not reach analytics %#v", response.Summary)
	}
}

func TestClientDisabledUsageIsNotOverridden(t *testing.T) {
	var seen string
	service, _ := newTestServiceWithConfigContents(t, koboldStreamBackend(t, &seen), map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true,"stream_options":{"include_usage":false}}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if requestAsksForStreamUsage(seen) {
		t.Fatalf("client opt-out was overridden, backend saw %s", seen)
	}
	if strings.Count(seen, "include_usage") != 1 {
		t.Fatalf("client stream_options was rewritten, backend saw %s", seen)
	}
	if strings.Contains(recorder.Body.String(), `"usage"`) {
		t.Fatalf("no usage chunk was requested %s", recorder.Body.String())
	}
}

func TestStreamUsageNotInjectedForBackendsThatAlreadyReport(t *testing.T) {
	body := []byte(`{"model":"llm","messages":[],"stream":true}`)
	injected, changed := injectStreamUsageOption(body, "/v1/chat/completions", BackendModeLlamaSDCPP)
	if changed || string(injected) != string(body) {
		t.Fatalf("llama already reports timings and must not be rewritten %s", injected)
	}
}

func TestStreamUsageNotInjectedForNonStreamingRequests(t *testing.T) {
	body := []byte(`{"model":"llm","messages":[]}`)
	injected, changed := injectStreamUsageOption(body, "/v1/chat/completions", BackendModeKobold)
	if changed || string(injected) != string(body) {
		t.Fatalf("non-streaming request must not be rewritten %s", injected)
	}
}

func TestStreamUsageInjectionKeepsRequestJSONValid(t *testing.T) {
	injected, changed := injectStreamUsageOption([]byte(`{"model":"llm","stream":true}`), "/v1/chat/completions", BackendModeKobold)
	if !changed {
		t.Fatal("expected injection")
	}
	if string(injected) != `{"model":"llm","stream":true,"stream_options":{"include_usage":true}}` {
		t.Fatalf("unexpected injected body %s", injected)
	}
}

func TestInjectedUsageFilterKeepsOtherChunks(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"a"}}]}`,
		"",
		koboldUsageChunk,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	filtered := readFilteredStream(t, stream)
	if strings.Contains(filtered, "usage") {
		t.Fatalf("usage chunk survived %s", filtered)
	}
	if !strings.Contains(filtered, `"content":"a"`) || !strings.Contains(filtered, "[DONE]") {
		t.Fatalf("filter dropped unrelated chunks %s", filtered)
	}
	if strings.Contains(filtered, "\n\n\n") {
		t.Fatalf("filter left a stray blank line %q", filtered)
	}
}

func TestInjectedUsageFilterKeepsChunksCarryingChoices(t *testing.T) {
	stream := `data: {"choices":[{"delta":{}}],"usage":{"total_tokens":9}}` + "\n\n"
	filtered := readFilteredStream(t, stream)
	if !strings.Contains(filtered, `"total_tokens":9`) {
		t.Fatalf("a chunk with choices must be preserved %s", filtered)
	}
}

func readFilteredStream(t *testing.T, stream string) string {
	t.Helper()
	response := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   io.NopCloser(strings.NewReader(stream)),
	}
	filtered := responseWithoutInjectedUsage(response, true)
	content, err := io.ReadAll(filtered.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
