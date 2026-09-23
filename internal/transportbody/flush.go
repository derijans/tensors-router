package transportbody

import (
	"io"
	"net/http"
)

func CopyFlushing(w http.ResponseWriter, src io.Reader) (int64, error) {
	buffer := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(buffer)
	flusher, flushes := w.(http.Flusher)
	var written int64
	for {
		read, readErr := src.Read(*buffer)
		if read > 0 {
			count, writeErr := w.Write((*buffer)[:read])
			written += int64(count)
			if flushes {
				flusher.Flush()
			}
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func CopyResponseFlushing(w http.ResponseWriter, src io.Reader, maxBytes int64) (int64, error) {
	return CopyFlushing(w, LimitResponse(src, maxBytes))
}
