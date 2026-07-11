package analytics

import (
	"io"
	"strings"
	"testing"
)

type recordingEventSink struct {
	events []Event
}

func (sink *recordingEventSink) Record(event Event) {
	sink.events = append(sink.events, event)
}

func TestResponseObserverCountsTransferredBytes(t *testing.T) {
	sink := &recordingEventSink{}
	observer := NewResponseObserver(sink, Event{}, "application/octet-stream", io.NopCloser(strings.NewReader("payload")))
	content, err := io.ReadAll(observer)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "payload" {
		t.Fatalf("unexpected response %q", content)
	}
	if len(sink.events) != 1 || sink.events[0].ResponseBytes != int64(len(content)) {
		t.Fatalf("unexpected recorded events %#v", sink.events)
	}
}
