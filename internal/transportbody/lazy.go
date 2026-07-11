package transportbody

import (
	"io"
	"sync"
)

type lazyTransformReadCloser struct {
	source    io.ReadCloser
	transform func(io.Reader, io.Writer) error
	startOnce sync.Once
	closeOnce sync.Once
	reader    *io.PipeReader
}

func newLazyTransformReadCloser(source io.ReadCloser, transform func(io.Reader, io.Writer) error) io.ReadCloser {
	return &lazyTransformReadCloser{source: source, transform: transform}
}

func (reader *lazyTransformReadCloser) Read(buffer []byte) (int, error) {
	reader.start()
	return reader.reader.Read(buffer)
}

func (reader *lazyTransformReadCloser) Close() error {
	var err error
	reader.closeOnce.Do(func() {
		if reader.reader != nil {
			_ = reader.reader.Close()
		}
		err = reader.source.Close()
	})
	return err
}

func (reader *lazyTransformReadCloser) start() {
	reader.startOnce.Do(func() {
		pipeReader, pipeWriter := io.Pipe()
		reader.reader = pipeReader
		go func() {
			err := reader.transform(reader.source, pipeWriter)
			_ = reader.source.Close()
			_ = pipeWriter.CloseWithError(err)
		}()
	})
}
