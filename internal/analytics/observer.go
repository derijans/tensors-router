package analytics

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
)

const observedBodyLimit = 1 << 20

type observedBodyKind int

const (
	observedBodyBuffered observedBodyKind = iota
	observedBodyEventStream
	observedBodyLineDelimitedJSON
)

type ResponseObserver struct {
	body           io.ReadCloser
	sink           EventSink
	event          Event
	contentType    string
	bodyKind       observedBodyKind
	finalizers     []func(*Event)
	lineBuffer     []byte
	bodyBuffer     bytes.Buffer
	firstContentAt time.Time
	lastContentAt  time.Time
	streamComplete bool
	once           sync.Once
}

func NewResponseObserver(sink EventSink, event Event, contentType string, body io.ReadCloser, finalizers ...func(*Event)) *ResponseObserver {
	return &ResponseObserver{
		body:        body,
		sink:        sink,
		event:       event,
		contentType: contentType,
		bodyKind:    observedBodyKindOf(contentType),
		finalizers:  append([]func(*Event){}, finalizers...),
	}
}

func (observer *ResponseObserver) Read(p []byte) (int, error) {
	read, readErr := observer.body.Read(p)
	if read > 0 {
		observer.event.ResponseBytes += int64(read)
		observer.observe(p[:read])
	}
	if readErr == io.EOF {
		observer.finish()
	}
	return read, readErr
}

func (observer *ResponseObserver) Close() error {
	err := observer.body.Close()
	observer.finish()
	return err
}

func (observer *ResponseObserver) observe(chunk []byte) {
	if observer.bodyKind == observedBodyBuffered {
		observer.bufferBody(chunk)
		return
	}
	observer.observeLines(chunk)
}

func (observer *ResponseObserver) bufferBody(chunk []byte) {
	if observer.bodyBuffer.Len() >= observedBodyLimit {
		return
	}
	remaining := observedBodyLimit - observer.bodyBuffer.Len()
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	_, _ = observer.bodyBuffer.Write(chunk)
}

func (observer *ResponseObserver) observeLines(chunk []byte) {
	observer.lineBuffer = append(observer.lineBuffer, chunk...)
	for {
		index := bytes.IndexByte(observer.lineBuffer, '\n')
		if index < 0 {
			if len(observer.lineBuffer) > observedBodyLimit {
				observer.lineBuffer = observer.lineBuffer[:0]
			}
			return
		}
		line := strings.TrimRight(string(observer.lineBuffer[:index]), "\r")
		observer.lineBuffer = observer.lineBuffer[index+1:]
		observer.observeLine(line)
	}
}

func (observer *ResponseObserver) observeLine(line string) {
	if observer.bodyKind == observedBodyLineDelimitedJSON {
		observer.observePayload([]byte(line))
		return
	}
	switch {
	case strings.HasPrefix(line, "data: "):
		observer.observePayload([]byte(strings.TrimPrefix(line, "data: ")))
	case strings.HasPrefix(line, "data:"):
		observer.observePayload([]byte(strings.TrimPrefix(line, "data:")))
	case looksLikeJSONObjectLine(line):
		observer.observePayload([]byte(line))
	}
}

func (observer *ResponseObserver) observePayload(payload []byte) {
	if bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		observer.streamComplete = true
		return
	}
	ApplyEventStreamData(&observer.event, payload)
	if !StreamPayloadCarriesContent(payload) {
		return
	}
	now := time.Now()
	if observer.firstContentAt.IsZero() {
		observer.firstContentAt = now
	} else if gap := now.Sub(observer.lastContentAt).Milliseconds(); gap > observer.event.MaxGapMS {
		observer.event.MaxGapMS = gap
	}
	observer.lastContentAt = now
}

func looksLikeJSONObjectLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), "{")
}

func (observer *ResponseObserver) finish() {
	observer.once.Do(func() {
		if len(observer.lineBuffer) > 0 {
			observer.observeLine(strings.TrimRight(string(observer.lineBuffer), "\r\n"))
			observer.lineBuffer = nil
		}
		now := time.Now()
		if observer.event.FinishedAt.IsZero() {
			observer.event.FinishedAt = now
		}
		if observer.event.StartedAt.IsZero() {
			observer.event.StartedAt = observer.event.FinishedAt
		}
		if observer.event.DurationMS == 0 {
			observer.event.DurationMS = observer.event.FinishedAt.Sub(observer.event.StartedAt).Milliseconds()
		}
		if observer.bodyKind == observedBodyBuffered {
			ApplyResponse(&observer.event, observer.contentType, observer.bodyBuffer.Bytes())
		}
		observer.applyStreamTimings()
		deriveTokenTotals(&observer.event)
		for _, finalizer := range observer.finalizers {
			if finalizer != nil {
				finalizer(&observer.event)
			}
		}
		observer.sink.Record(observer.event)
	})
}

func (observer *ResponseObserver) applyStreamTimings() {
	if observer.firstContentAt.IsZero() {
		return
	}
	if !observer.event.StartedAt.IsZero() {
		observer.event.TTFTMS = observer.firstContentAt.Sub(observer.event.StartedAt).Milliseconds()
	}
	observer.event.DecodeMS = observer.lastContentAt.Sub(observer.firstContentAt).Milliseconds()
	observer.event.Aborted = !observer.streamComplete && observer.event.FinishReason == ""
}

func observedBodyKindOf(contentType string) observedBodyKind {
	lowered := strings.ToLower(contentType)
	switch {
	case strings.Contains(lowered, "text/event-stream"):
		return observedBodyEventStream
	case strings.Contains(lowered, "application/x-ndjson"), strings.Contains(lowered, "application/ndjson"):
		return observedBodyLineDelimitedJSON
	default:
		return observedBodyBuffered
	}
}
