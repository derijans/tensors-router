package transportbody

import (
	"io"
	"sync"
	"sync/atomic"
)

type Body interface {
	OpenAttempt() (*Attempt, error)
	Replayable() bool
	CanRetry() bool
	Size() (int64, bool)
	BytesConsumed() int64
	Close() error
}

type Attempt struct {
	reader    io.Reader
	closeOnce sync.Once
	close     func() error
	readBytes atomic.Int64
	observed  func() int64
}

func (attempt *Attempt) Read(buffer []byte) (int, error) {
	read, err := attempt.reader.Read(buffer)
	if attempt.observed == nil && read > 0 {
		attempt.readBytes.Add(int64(read))
	}
	return read, err
}

func (attempt *Attempt) Close() error {
	var err error
	attempt.closeOnce.Do(func() {
		if attempt.close != nil {
			err = attempt.close()
		}
	})
	return err
}

func (attempt *Attempt) BytesRead() int64 {
	if attempt.observed != nil {
		return attempt.observed()
	}
	return attempt.readBytes.Load()
}

type replayBody struct {
	chunks       [][]byte
	size         int64
	reservations []*Reservation
	closed       atomic.Bool
	consumed     atomic.Int64
}

func newReplayBody(chunks [][]byte, size int64, reservations []*Reservation) Body {
	return &replayBody{chunks: chunks, size: size, reservations: reservations}
}

func (body *replayBody) OpenAttempt() (*Attempt, error) {
	if body.closed.Load() {
		return nil, ErrTransportBodyClosed
	}
	attempt := &Attempt{reader: &chunkReader{chunks: body.chunks}}
	attempt.close = func() error {
		body.consumed.Add(attempt.readBytes.Load())
		return nil
	}
	return attempt, nil
}

func (body *replayBody) Replayable() bool {
	return true
}

func (body *replayBody) CanRetry() bool {
	return !body.closed.Load()
}

func (body *replayBody) Size() (int64, bool) {
	return body.size, true
}

func (body *replayBody) BytesConsumed() int64 {
	return body.consumed.Load()
}

func (body *replayBody) Close() error {
	if !body.closed.CompareAndSwap(false, true) {
		return nil
	}
	for _, reservation := range body.reservations {
		reservation.Release()
	}
	body.chunks = nil
	return nil
}

type streamingBody struct {
	mu           sync.Mutex
	prefix       [][]byte
	source       io.ReadCloser
	maxBytes     int64
	knownSize    int64
	sizeKnown    bool
	active       bool
	closed       bool
	consumed     int64
	reservations []*Reservation
}

func newStreamingBody(prefix [][]byte, source io.ReadCloser, maxBytes int64, knownSize int64, sizeKnown bool, reservations []*Reservation) Body {
	return &streamingBody{
		prefix:       prefix,
		source:       source,
		maxBytes:     maxBytes,
		knownSize:    knownSize,
		sizeKnown:    sizeKnown,
		reservations: reservations,
	}
}

func (body *streamingBody) OpenAttempt() (*Attempt, error) {
	body.mu.Lock()
	if body.closed {
		body.mu.Unlock()
		return nil, ErrTransportBodyClosed
	}
	if body.active {
		body.mu.Unlock()
		return nil, ErrStreamingBodyInUse
	}
	if body.consumed > 0 {
		body.mu.Unlock()
		return nil, ErrStreamingBodyConsumed
	}
	body.active = true
	body.mu.Unlock()

	reader := io.MultiReader(&chunkReader{chunks: body.prefix}, body.source)
	limited := &requestLimitReader{reader: reader, remaining: body.maxBytes}
	attempt := &Attempt{reader: limited}
	attempt.close = func() error {
		body.mu.Lock()
		body.active = false
		body.consumed += attempt.readBytes.Load()
		body.mu.Unlock()
		return nil
	}
	return attempt, nil
}

func (body *streamingBody) Replayable() bool {
	return false
}

func (body *streamingBody) CanRetry() bool {
	body.mu.Lock()
	defer body.mu.Unlock()
	return !body.closed && !body.active && body.consumed == 0
}

func (body *streamingBody) Size() (int64, bool) {
	return body.knownSize, body.sizeKnown
}

func (body *streamingBody) BytesConsumed() int64 {
	body.mu.Lock()
	defer body.mu.Unlock()
	return body.consumed
}

func (body *streamingBody) Close() error {
	body.mu.Lock()
	if body.closed {
		body.mu.Unlock()
		return nil
	}
	body.closed = true
	source := body.source
	body.source = nil
	body.prefix = nil
	body.mu.Unlock()
	for _, reservation := range body.reservations {
		reservation.Release()
	}
	if source != nil {
		return source.Close()
	}
	return nil
}

type chunkReader struct {
	chunks [][]byte
	index  int
	offset int
}

func (reader *chunkReader) Read(buffer []byte) (int, error) {
	for reader.index < len(reader.chunks) {
		chunk := reader.chunks[reader.index]
		if reader.offset >= len(chunk) {
			reader.index++
			reader.offset = 0
			continue
		}
		read := copy(buffer, chunk[reader.offset:])
		reader.offset += read
		return read, nil
	}
	return 0, io.EOF
}

type requestLimitReader struct {
	reader    io.Reader
	remaining int64
	exceeded  bool
}

func (reader *requestLimitReader) Read(buffer []byte) (int, error) {
	if reader.exceeded {
		return 0, ErrRequestTooLarge
	}
	if reader.remaining > 0 {
		if int64(len(buffer)) > reader.remaining {
			buffer = buffer[:reader.remaining]
		}
		read, err := reader.reader.Read(buffer)
		reader.remaining -= int64(read)
		return read, err
	}
	var probe [1]byte
	read, err := reader.reader.Read(probe[:])
	if read > 0 {
		reader.exceeded = true
		return 0, ErrRequestTooLarge
	}
	return 0, err
}

type transformedBody struct {
	base      Body
	factory   func(io.ReadCloser) (io.ReadCloser, error)
	size      int64
	sizeKnown bool
}

func Transform(base Body, size int64, sizeKnown bool, factory func(io.ReadCloser) (io.ReadCloser, error)) Body {
	return &transformedBody{base: base, factory: factory, size: size, sizeKnown: sizeKnown}
}

func (body *transformedBody) OpenAttempt() (*Attempt, error) {
	baseAttempt, err := body.base.OpenAttempt()
	if err != nil {
		return nil, err
	}
	reader, err := body.factory(baseAttempt)
	if err != nil {
		_ = baseAttempt.Close()
		return nil, err
	}
	return &Attempt{
		reader:   reader,
		close:    reader.Close,
		observed: baseAttempt.BytesRead,
	}, nil
}

func (body *transformedBody) Replayable() bool {
	return body.base.Replayable()
}

func (body *transformedBody) CanRetry() bool {
	return body.base.CanRetry()
}

func (body *transformedBody) Size() (int64, bool) {
	return body.size, body.sizeKnown
}

func (body *transformedBody) BytesConsumed() int64 {
	return body.base.BytesConsumed()
}

func (body *transformedBody) Close() error {
	return body.base.Close()
}
