package routertls

import (
	"net/http"
	"sync"
)

type PeerTransport struct {
	runtime  *Runtime
	mu       sync.Mutex
	revision int64
	current  *http.Transport
}

func (runtime *Runtime) PeerTransport() *PeerTransport {
	return &PeerTransport{runtime: runtime}
}

func (transport *PeerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport.transportForCurrentMaterial().RoundTrip(request)
}

func (transport *PeerTransport) transportForCurrentMaterial() *http.Transport {
	material := transport.runtime.current()
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.current != nil && transport.revision == material.revision {
		return transport.current
	}
	previous := transport.current
	next := http.DefaultTransport.(*http.Transport).Clone()
	next.TLSClientConfig = material.client.Clone()
	transport.current = next
	transport.revision = material.revision
	if previous != nil {
		previous.CloseIdleConnections()
	}
	return next
}

func (transport *PeerTransport) CloseIdleConnections() {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.current != nil {
		transport.current.CloseIdleConnections()
	}
}
