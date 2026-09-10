package analytics

import (
	"io"
	"strings"
	"testing"
)

func TestResponseObserverReadsCountsFromFinalNDJSONRecord(t *testing.T) {
	body := strings.Join([]string{
		`{"model":"m","response":"he","done":false}`,
		`{"model":"m","response":"llo","done":false}`,
		`{"model":"m","response":"","done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":153}`,
		"",
	}, "\n")
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{DurationMS: 6680}, "application/x-ndjson", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if len(sink.events) != 1 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
	recorded := sink.events[0]
	if recorded.InputTokens != 11 || recorded.OutputTokens != 153 || recorded.TotalTokens != 164 {
		t.Fatalf("unexpected token counts %#v", recorded)
	}
}

func TestResponseObserverReadsCountsFromUnterminatedNDJSONRecord(t *testing.T) {
	body := `{"model":"m","response":"hi","done":false}` + "\n" +
		`{"model":"m","done":true,"prompt_eval_count":7,"eval_count":9}`
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{DurationMS: 1000}, "application/x-ndjson", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if len(sink.events) != 1 || sink.events[0].InputTokens != 7 || sink.events[0].OutputTokens != 9 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
}

func TestResponseObserverReadsUsageFromFinalEventStreamChunk(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		"",
		`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":153,"total_tokens":164}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{DurationMS: 6680}, "text/event-stream", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if len(sink.events) != 1 || sink.events[0].OutputTokens != 153 || sink.events[0].TotalTokens != 164 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
}

func TestResponseObserverReadsBareJSONLinesLabelledAsEventStream(t *testing.T) {
	body := strings.Join([]string{
		`{"model":"m","response":"he","done":false}`,
		`{"model":"m","response":"llo","done":false}`,
		`{"model":"m","response":"","done":true,"done_reason":"stop","prompt_eval_count":11,"prompt_eval_duration":1,"eval_count":11,"eval_duration":1}`,
		"",
	}, "\n")
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{DurationMS: 480}, "text/event-stream", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if len(sink.events) != 1 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
	recorded := sink.events[0]
	if recorded.InputTokens != 11 || recorded.OutputTokens != 11 {
		t.Fatalf("unexpected token counts %#v", recorded)
	}
}

func TestResponseObserverIgnoresSSEFieldLines(t *testing.T) {
	body := strings.Join([]string{
		"event: message",
		`data: {"token":"hi","finish_reason":null}`,
		"",
		"id: 7",
		"retry: 100",
		": comment",
		`data: {"choices":[],"usage":{"completion_tokens":5}}`,
		"",
	}, "\n")
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{DurationMS: 1000}, "text/event-stream", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if len(sink.events) != 1 || sink.events[0].OutputTokens != 5 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
}

func TestResponseObserverIgnoresNonJSONLinesInNDJSONBody(t *testing.T) {
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{DurationMS: 1000}, "application/x-ndjson", io.NopCloser(strings.NewReader("not json\n\n{\"eval_count\":4}\n")))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if len(sink.events) != 1 || sink.events[0].OutputTokens != 4 {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
}
