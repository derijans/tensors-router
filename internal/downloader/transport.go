package downloader

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"tensors-router/internal/buildinfo"
)

const (
	connectTimeout      = 30 * time.Second
	idleConnectionLimit = 90 * time.Second
)

var errDownloadStalled = errors.New("download stalled: no data received within the stall timeout")

func newHubTransport(responseHeaderTimeout time.Duration) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = connectTimeout
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	transport.IdleConnTimeout = idleConnectionLimit
	return transport
}

func userAgent() string {
	return "tensors-router-downloader/" + buildinfo.Current().Version
}

type stallGuard struct {
	timer   *time.Timer
	timeout time.Duration
	stalled atomic.Bool
}

func guardAgainstStall(ctx context.Context, timeout time.Duration) (context.Context, *stallGuard, context.CancelFunc) {
	guarded, cancel := context.WithCancel(ctx)
	guard := &stallGuard{timeout: timeout}
	guard.timer = time.AfterFunc(timeout, func() {
		guard.stalled.Store(true)
		cancel()
	})
	return guarded, guard, func() {
		guard.timer.Stop()
		cancel()
	}
}

func (guard *stallGuard) reader(source io.Reader) io.Reader {
	return progressResettingReader{source: source, guard: guard}
}

func (guard *stallGuard) explain(err error) error {
	if err != nil && guard.stalled.Load() {
		return errDownloadStalled
	}
	return err
}

type progressResettingReader struct {
	source io.Reader
	guard  *stallGuard
}

func (reader progressResettingReader) Read(buffer []byte) (int, error) {
	count, err := reader.source.Read(buffer)
	if count > 0 {
		reader.guard.timer.Reset(reader.guard.timeout)
	}
	return count, err
}
