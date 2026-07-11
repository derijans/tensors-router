package transportbody

import (
	"io"
	"sync"
)

const CopyBufferBytes = 256 * 1024

var copyBufferPool = sync.Pool{
	New: func() any {
		buffer := make([]byte, CopyBufferBytes)
		return &buffer
	},
}

func Copy(dst io.Writer, src io.Reader) (int64, error) {
	buffer := copyBufferPool.Get().(*[]byte)
	written, err := io.CopyBuffer(dst, src, *buffer)
	copyBufferPool.Put(buffer)
	return written, err
}

func CopyResponse(dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultLimits().MaxResponseBytes
	}
	limited := LimitResponse(src, maxBytes)
	written, err := Copy(dst, limited)
	if err != nil {
		return written, err
	}
	return written, nil
}

func LimitResponse(src io.Reader, maxBytes int64) io.Reader {
	if maxBytes <= 0 {
		maxBytes = DefaultLimits().MaxResponseBytes
	}
	return &responseLimitReader{reader: src, remaining: maxBytes}
}

type responseLimitReader struct {
	reader    io.Reader
	remaining int64
	exceeded  bool
}

func (reader *responseLimitReader) Read(buffer []byte) (int, error) {
	if reader.exceeded {
		return 0, ErrResponseTooLarge
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
		return 0, ErrResponseTooLarge
	}
	return 0, err
}
