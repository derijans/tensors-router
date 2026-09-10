package analytics

import (
	"io"
	"strings"
	"testing"
	"time"
)

type pacedStream struct {
	chunks []string
	delay  time.Duration
	index  int
}

func (stream *pacedStream) Read(p []byte) (int, error) {
	if stream.index >= len(stream.chunks) {
		return 0, io.EOF
	}
	time.Sleep(stream.delay)
	chunk := stream.chunks[stream.index]
	stream.index++
	return copy(p, chunk), nil
}

func (stream *pacedStream) Close() error { return nil }

func TestResponseObserverMeasuresStreamTimings(t *testing.T) {
	sink := &recordingEventSink{}
	started := time.Now()
	stream := &pacedStream{
		delay: 25 * time.Millisecond,
		chunks: []string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":"one"}}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":"two"}}]}` + "\n\n",
			`data: {"choices":[{"finish_reason":"stop","delta":{}}]}` + "\n\n",
			"data: [DONE]\n\n",
		},
	}
	observer := NewResponseObserver(sink, Event{StartedAt: started}, "text/event-stream", stream)
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	recorded := sink.events[0]
	if recorded.TTFTMS <= 0 {
		t.Fatalf("ttft was not measured %#v", recorded)
	}
	if recorded.DecodeMS <= 0 {
		t.Fatalf("decode time was not measured %#v", recorded)
	}
	if recorded.TTFTMS+recorded.DecodeMS > recorded.DurationMS+5 {
		t.Fatalf("ttft %d + decode %d exceeds duration %d", recorded.TTFTMS, recorded.DecodeMS, recorded.DurationMS)
	}
	if recorded.MaxGapMS <= 0 {
		t.Fatalf("max gap was not measured %#v", recorded)
	}
	if recorded.FinishReason != "stop" {
		t.Fatalf("unexpected finish reason %q", recorded.FinishReason)
	}
	if recorded.Aborted {
		t.Fatalf("a completed stream must not be marked aborted %#v", recorded)
	}
}

func TestResponseObserverSkipsTimingsForNonStreamingResponses(t *testing.T) {
	sink := &recordingEventSink{}
	body := `{"usage":{"prompt_tokens":3,"completion_tokens":4},"choices":[{"finish_reason":"length","message":{"content":"hi"}}]}`
	observer := NewResponseObserver(sink, Event{StartedAt: time.Now()}, "application/json", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	recorded := sink.events[0]
	if recorded.TTFTMS != 0 || recorded.DecodeMS != 0 || recorded.MaxGapMS != 0 {
		t.Fatalf("non-streaming response must not carry stream timings %#v", recorded)
	}
	if recorded.FinishReason != "length" {
		t.Fatalf("unexpected finish reason %q", recorded.FinishReason)
	}
	if recorded.Aborted {
		t.Fatalf("non-streaming response must not be marked aborted %#v", recorded)
	}
}

func TestResponseObserverMarksTruncatedStreamAborted(t *testing.T) {
	sink := &recordingEventSink{}
	body := `data: {"choices":[{"delta":{"content":"one"}}]}` + "\n\n"
	observer := NewResponseObserver(sink, Event{StartedAt: time.Now()}, "text/event-stream", io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	if !sink.events[0].Aborted {
		t.Fatalf("a stream without a finish reason or [DONE] is aborted %#v", sink.events[0])
	}
}

func TestResponseObserverMeasuresKoboldNativeStreamTimings(t *testing.T) {
	sink := &recordingEventSink{}
	stream := &pacedStream{
		delay: 20 * time.Millisecond,
		chunks: []string{
			"event: message\n" + `data: {"token": " One", "finish_reason": null}` + "\n\n",
			"event: message\n" + `data: {"token": " two", "finish_reason": null}` + "\n\n",
			"event: message\n" + `data: {"token": "", "finish_reason": "stop"}` + "\n\n",
		},
	}
	observer := NewResponseObserver(sink, Event{StartedAt: time.Now()}, "text/event-stream", stream)
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	recorded := sink.events[0]
	if recorded.TTFTMS <= 0 || recorded.DecodeMS <= 0 {
		t.Fatalf("kobold native stream timings were not measured %#v", recorded)
	}
	if recorded.FinishReason != "stop" {
		t.Fatalf("unexpected finish reason %q", recorded.FinishReason)
	}
	if recorded.Aborted {
		t.Fatalf("finish reason present, so the stream is not aborted %#v", recorded)
	}
}

func TestStreamPayloadCarriesContent(t *testing.T) {
	carrying := []string{
		`{"choices":[{"delta":{"content":"hi"}}]}`,
		`{"choices":[{"text":"hi"}]}`,
		`{"token":"hi"}`,
		`{"response":"hi"}`,
		`{"delta":{"text":"hi"}}`,
	}
	for _, payload := range carrying {
		if !StreamPayloadCarriesContent([]byte(payload)) {
			t.Errorf("expected content in %s", payload)
		}
	}
	empty := []string{
		"[DONE]",
		`{"choices":[{"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"finish_reason":"stop","delta":{}}]}`,
		`{"choices":[],"usage":{"total_tokens":3}}`,
		`{"token":""}`,
	}
	for _, payload := range empty {
		if StreamPayloadCarriesContent([]byte(payload)) {
			t.Errorf("expected no content in %s", payload)
		}
	}
}
