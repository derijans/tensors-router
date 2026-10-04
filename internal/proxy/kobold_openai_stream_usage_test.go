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

const koboldToolCallFakeStreamBody = `data: {"id": "chatcmpl-A1", "object": "chat.completion.chunk", "created": 1759590000, "model": "llm", "choices": [{"index": 0, "finish_reason": null, "delta": {"role": "assistant", "content": ""}}]}` + "\n\n" +
	`data: {"id": "koboldcpp", "object": "chat.completion.chunk", "created": 1759590000, "model": "llm", "choices": [{"index": 0, "finish_reason": null, "delta": {"tool_calls": [{"index": 0, "id": "call_47013", "type": "function", "function": {"name": "get_weather", "arguments": ""}}]}}]}` + "\n\n" +
	`data: {"id": "koboldcpp", "object": "chat.completion.chunk", "created": 1759590000, "model": "llm", "choices": [{"index": 0, "finish_reason": null, "delta": {"tool_calls": [{"index": 0, "function": {"arguments": "{\"city\": \"Tallinn\"}"}}]}}]}` + "\n\n" +
	`data: {"id": "koboldcpp", "object": "chat.completion.chunk", "created": 1759590000, "model": "llm", "choices": [{"index": 0, "finish_reason": "tool_calls", "delta": {}}]}` + "\n\n" +
	"data: [DONE]\n\n"

const koboldStreamDoneEvent = "data: [DONE]\n\n"

type koboldOpenAIStreamBackend struct {
	streamBody           string
	generationsPerStream int64
	totalGenerations     atomic.Int64
}

func (backend *koboldOpenAIStreamBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/extra/perf":
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"last_input_count": 250, "last_token_count": 22, "last_process_time": 6.75, "last_process_speed": 37.0, "total_gens": %d}`, backend.totalGenerations.Load())
	case "/v1/chat/completions":
		backend.totalGenerations.Add(backend.generationsPerStream)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(backend.streamBody))
	default:
		http.NotFound(w, r)
	}
}

func serveKoboldOpenAIStream(t *testing.T, backend *koboldOpenAIStreamBackend, requestBody string) (*httptest.ResponseRecorder, routeranalytics.Response) {
	t.Helper()
	service, _ := newTestServiceWithConfigContents(t, backend, map[string]string{
		"llm": `{"model_param":"llm.gguf"}`,
	})
	service.analytics.store = newProxyAnalyticsStore(t, "local")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	return recorder, queryProxyAnalytics(t, service.analytics.store)
}

const toolCallStreamRequest = `{"model":"llm","stream":true,"messages":[{"role":"user","content":"weather in Tallinn?"}],"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}]}`

func TestKoboldToolCallStreamRecordsGenerationCountsTheClientNeverSees(t *testing.T) {
	recorder, response := serveKoboldOpenAIStream(t, &koboldOpenAIStreamBackend{streamBody: koboldToolCallFakeStreamBody, generationsPerStream: 1}, toolCallStreamRequest)

	if recorder.Body.String() != koboldToolCallFakeStreamBody {
		t.Fatalf("client stream was altered %q", recorder.Body.String())
	}
	if response.Summary.InputTokens != 250 || response.Summary.OutputTokens != 22 || response.Summary.TotalTokens != 272 {
		t.Fatalf("kobold tool call stream counts were not recorded %#v", response.Summary)
	}
	event := recentEventOfType(t, response, routeranalytics.EventTypeRequest)
	if event.FinishReason != "tool_calls" || event.Aborted {
		t.Fatalf("tool call stream completion was misread %#v", event)
	}
}

func TestKoboldToolCallStreamGivesTheClientTheUsageItAskedFor(t *testing.T) {
	requestBody := strings.Replace(toolCallStreamRequest, `"stream":true`, `"stream":true,"stream_options":{"include_usage":true}`, 1)
	recorder, response := serveKoboldOpenAIStream(t, &koboldOpenAIStreamBackend{streamBody: koboldToolCallFakeStreamBody, generationsPerStream: 1}, requestBody)

	body := recorder.Body.String()
	usageEvent := `data: {"id":"koboldcpp","object":"chat.completion.chunk","created":1759590000,"model":"llm","choices":[],"usage":{"prompt_tokens":250,"completion_tokens":22,"total_tokens":272}}` + "\n\n"
	expected := strings.TrimSuffix(koboldToolCallFakeStreamBody, koboldStreamDoneEvent) + usageEvent + koboldStreamDoneEvent
	if body != expected {
		t.Fatalf("client did not get one complete usage chunk before [DONE]\n got %q\nwant %q", body, expected)
	}
	if response.Summary.InputTokens != 250 || response.Summary.OutputTokens != 22 {
		t.Fatalf("kobold tool call stream counts were not recorded %#v", response.Summary)
	}
	if event := recentEventOfType(t, response, routeranalytics.EventTypeRequest); event.PromptTokensPS != 37 {
		t.Fatalf("prompt processing speed was not recorded %#v", event)
	}
}

func TestKoboldStreamThatReportsUsageKeepsItsOwnCounts(t *testing.T) {
	streamWithUsage := `data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hi"}}]}` + "\n\n" +
		`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"finish_reason":"stop","delta":{}}]}` + "\n\n" +
		koboldUsageChunk + "\n\n" +
		koboldStreamDoneEvent
	requestBody := `{"model":"llm","messages":[],"stream":true,"stream_options":{"include_usage":true}}`
	recorder, response := serveKoboldOpenAIStream(t, &koboldOpenAIStreamBackend{streamBody: streamWithUsage, generationsPerStream: 1}, requestBody)

	if recorder.Body.String() != streamWithUsage {
		t.Fatalf("a stream that already reports usage was altered %q", recorder.Body.String())
	}
	if response.Summary.InputTokens != 13 || response.Summary.OutputTokens != 11 {
		t.Fatalf("the backend's own usage was not kept %#v", response.Summary)
	}
}

func TestKoboldToolCallStreamLeavesCountsUnknownWhenAnotherGenerationFinishedAlongside(t *testing.T) {
	recorder, response := serveKoboldOpenAIStream(t, &koboldOpenAIStreamBackend{streamBody: koboldToolCallFakeStreamBody, generationsPerStream: 2}, toolCallStreamRequest)

	if recorder.Body.String() != koboldToolCallFakeStreamBody {
		t.Fatalf("client stream was altered %q", recorder.Body.String())
	}
	if response.Summary.RequestCount != 1 {
		t.Fatalf("kobold tool call stream was not recorded %#v", response.Summary)
	}
	if response.Summary.InputTokens != 0 || response.Summary.OutputTokens != 0 {
		t.Fatalf("counts of a generation that may not be this request's were attributed %#v", response.Summary)
	}
}
