package analytics

import (
	"io"
	"strings"
	"testing"
)

func observedFailure(t *testing.T, status int, contentType string, body string) Event {
	t.Helper()
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{StatusCode: status}, contentType, io.NopCloser(strings.NewReader(body)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(sink.events))
	}
	return sink.events[0]
}

func TestUpstreamErrorMessageIsKeptFromAnOpenAIErrorBody(t *testing.T) {
	event := observedFailure(t, 400, "application/json", `{"error":{"code":400,"message":"image input is not supported - hint: if this is unexpected, you may need to provide the mmproj","type":"invalid_request_error"}}`)

	if !strings.Contains(event.ErrorMessage, "image input is not supported") {
		t.Fatalf("error message = %q, want the upstream reason", event.ErrorMessage)
	}
	if event.UpstreamStatus != 400 {
		t.Fatalf("upstream status = %d, want 400", event.UpstreamStatus)
	}
}

func TestUpstreamErrorMessageIsKeptFromAPlainStringError(t *testing.T) {
	event := observedFailure(t, 422, "application/json", `{"error":"model is not multimodal"}`)

	if event.ErrorMessage != "model is not multimodal" {
		t.Fatalf("error message = %q", event.ErrorMessage)
	}
}

func TestUpstreamErrorMessageFallsBackToTheBodyText(t *testing.T) {
	event := observedFailure(t, 500, "text/plain", "backend exploded")

	if event.ErrorMessage != "backend exploded" {
		t.Fatalf("error message = %q", event.ErrorMessage)
	}
}

func TestSuccessfulResponseCarriesNoErrorMessage(t *testing.T) {
	event := observedFailure(t, 200, "application/json", `{"error":"not really an error"}`)

	if event.ErrorMessage != "" || event.UpstreamStatus != 0 {
		t.Fatalf("event = %q / %d, want no error recorded for a 200", event.ErrorMessage, event.UpstreamStatus)
	}
}
