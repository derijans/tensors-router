package loadcapture

import (
	"io"
	"sync"
	"time"
)

type Stream string

const maxChunkBytes = 256 * 1024

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
)

type Chunk struct {
	Sequence int64         `json:"sequence"`
	Stream   Stream        `json:"stream"`
	Offset   time.Duration `json:"offset_ns"`
	Payload  []byte        `json:"payload"`
}

type Capture struct {
	Chunks        []Chunk
	CapturedBytes int64
	Truncated     bool
	Secrets       []string
}

type Hub struct {
	mu          sync.Mutex
	started     time.Time
	sequence    int64
	subscribers map[*subscription]struct{}
	watchers    map[*watcher]struct{}
}

type watcher struct {
	observe func(Stream, []byte)
}

type subscription struct {
	limit   int64
	capture Capture
	closed  bool
}

type writer struct {
	hub    *Hub
	stream Stream
}

func NewHub() *Hub {
	return &Hub{started: time.Now(), subscribers: make(map[*subscription]struct{}), watchers: make(map[*watcher]struct{})}
}

// Watch delivers every captured chunk to observe as it arrives, for callers that need
// to react to backend output live rather than collect it. Unlike Subscribe there is no
// byte budget: nothing is retained. observe runs while the hub lock is held, so it must
// not block or call back into the hub.
func (hub *Hub) Watch(observe func(Stream, []byte)) func() {
	if hub == nil || observe == nil {
		return func() {}
	}
	entry := &watcher{observe: observe}
	hub.mu.Lock()
	hub.watchers[entry] = struct{}{}
	hub.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			hub.mu.Lock()
			delete(hub.watchers, entry)
			hub.mu.Unlock()
		})
	}
}

func (hub *Hub) Stdout() io.Writer {
	return writer{hub: hub, stream: StreamStdout}
}

func (hub *Hub) Stderr() io.Writer {
	return writer{hub: hub, stream: StreamStderr}
}

func (hub *Hub) Subscribe(limit int64) func() Capture {
	if hub == nil {
		return func() Capture { return Capture{} }
	}
	if limit < 1 {
		limit = 1
	}
	subscriber := &subscription{limit: limit}
	hub.mu.Lock()
	hub.subscribers[subscriber] = struct{}{}
	hub.mu.Unlock()
	var once sync.Once
	return func() Capture {
		once.Do(func() {
			hub.mu.Lock()
			delete(hub.subscribers, subscriber)
			subscriber.closed = true
			hub.mu.Unlock()
		})
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return cloneCapture(subscriber.capture)
	}
}

func (writer writer) Write(payload []byte) (int, error) {
	if len(payload) == 0 || writer.hub == nil {
		return len(payload), nil
	}
	writer.hub.record(writer.stream, payload)
	return len(payload), nil
}

func (hub *Hub) record(stream Stream, payload []byte) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for entry := range hub.watchers {
		entry.observe(stream, payload)
	}
	if len(hub.subscribers) == 0 {
		return
	}
	offset := time.Since(hub.started)
	for start := 0; start < len(payload); start += maxChunkBytes {
		end := min(start+maxChunkBytes, len(payload))
		hub.sequence++
		for subscriber := range hub.subscribers {
			remaining := subscriber.limit - subscriber.capture.CapturedBytes
			if remaining <= 0 {
				subscriber.capture.Truncated = true
				continue
			}
			captured := payload[start:end]
			if int64(len(captured)) > remaining {
				captured = captured[:remaining]
				subscriber.capture.Truncated = true
			}
			subscriber.capture.Chunks = append(subscriber.capture.Chunks, Chunk{
				Sequence: hub.sequence,
				Stream:   stream,
				Offset:   offset,
				Payload:  append([]byte(nil), captured...),
			})
			subscriber.capture.CapturedBytes += int64(len(captured))
		}
	}
}

func cloneCapture(capture Capture) Capture {
	cloned := Capture{CapturedBytes: capture.CapturedBytes, Truncated: capture.Truncated, Chunks: make([]Chunk, len(capture.Chunks)), Secrets: append([]string(nil), capture.Secrets...)}
	for index, chunk := range capture.Chunks {
		cloned.Chunks[index] = Chunk{Sequence: chunk.Sequence, Stream: chunk.Stream, Offset: chunk.Offset, Payload: append([]byte(nil), chunk.Payload...)}
	}
	return cloned
}
