package analytics

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
)

const observedBodyLimit = 1 << 20

type ResponseObserver struct {
	body        io.ReadCloser
	sink        EventSink
	event       Event
	contentType string
	lineBuffer  []byte
	bodyBuffer  bytes.Buffer
	once        sync.Once
}

func NewResponseObserver(sink EventSink, event Event, contentType string, body io.ReadCloser) *ResponseObserver {
	return &ResponseObserver{
		body:        body,
		sink:        sink,
		event:       event,
		contentType: contentType,
	}
}

func (observer *ResponseObserver) Read(p []byte) (int, error) {
	read, readErr := observer.body.Read(p)
	if read > 0 {
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
	if isEventStreamContent(observer.contentType) {
		observer.observeEventStream(chunk)
		return
	}
	if observer.bodyBuffer.Len() >= observedBodyLimit {
		return
	}
	remaining := observedBodyLimit - observer.bodyBuffer.Len()
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	_, _ = observer.bodyBuffer.Write(chunk)
}

func (observer *ResponseObserver) observeEventStream(chunk []byte) {
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
		observer.observeEventStreamLine(line)
	}
}

func (observer *ResponseObserver) observeEventStreamLine(line string) {
	switch {
	case strings.HasPrefix(line, "data: "):
		ApplyEventStreamData(&observer.event, []byte(strings.TrimPrefix(line, "data: ")))
	case strings.HasPrefix(line, "data:"):
		ApplyEventStreamData(&observer.event, []byte(strings.TrimPrefix(line, "data:")))
	}
}

func (observer *ResponseObserver) finish() {
	observer.once.Do(func() {
		if len(observer.lineBuffer) > 0 {
			observer.observeEventStreamLine(strings.TrimRight(string(observer.lineBuffer), "\r\n"))
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
		if !isEventStreamContent(observer.contentType) {
			ApplyResponse(&observer.event, observer.contentType, observer.bodyBuffer.Bytes())
		}
		observer.sink.Record(observer.event)
	})
}

func isEventStreamContent(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
