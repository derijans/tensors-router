package backendendpoint

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"tensors-router/internal/portalloc"
)

// Endpoint is a backend process's loopback address. It supports three forms
// on construction, chosen by the port in the configured URL:
//
//   - an explicit port ("http://127.0.0.1:5001") is pinned: the endpoint
//     never changes and Reserve/Release are no-ops.
//   - port 0 ("http://127.0.0.1:0") is dynamic: the router allocates a free
//     loopback port at spawn time via Reserve, and the endpoint reports
//     port "0" (an address nothing can successfully dial) until then.
//   - a missing port is rejected at construction. Earlier versions of this
//     router silently fell back to a hardcoded port shared by every
//     manager, which made a misconfigured embeddings endpoint quietly
//     alias the primary backend; requiring an explicit port or "0" removes
//     that failure mode entirely.
//
// Endpoint is safe for concurrent use. Callers that read the address
// (URL, HostPort) may run concurrently with a caller reserving or
// releasing it; Endpoint's own mutex guards that, independent of and
// never nested inside any caller-held lock.
type Endpoint struct {
	mu       sync.RWMutex
	template url.URL
	host     string
	pinned   string // fixed port; empty when dynamic
	port     string // current port; "0" for a dynamic endpoint that has never been reserved
}

// NewEndpoint parses rawURL as a loopback backend address. See the Endpoint
// documentation for the meaning of an explicit port, port 0, and a missing
// port.
func NewEndpoint(rawURL string) (*Endpoint, error) {
	parsed, err := ParseLoopback(rawURL)
	if err != nil {
		return nil, err
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	port := parsed.Port()
	if port == "" {
		return nil, fmt.Errorf("backend URL %q must include an explicit port, or 0 to allocate one at startup", rawURL)
	}
	template := *parsed
	template.Host = ""

	endpoint := &Endpoint{template: template, host: host, port: port}
	if port != "0" {
		endpoint.pinned = port
	}
	return endpoint, nil
}

// Dynamic reports whether the endpoint's port is router-allocated rather
// than pinned by configuration.
func (endpoint *Endpoint) Dynamic() bool {
	endpoint.mu.RLock()
	defer endpoint.mu.RUnlock()
	return endpoint.pinned == ""
}

// URL returns the endpoint's current address. For a dynamic endpoint that
// has not yet been reserved, this is a port-0 URL that fails to dial
// immediately rather than silently reaching some other backend.
func (endpoint *Endpoint) URL() *url.URL {
	endpoint.mu.RLock()
	result := endpoint.template
	result.Host = net.JoinHostPort(endpoint.host, endpoint.port)
	endpoint.mu.RUnlock()
	return &result
}

// HostPort returns the endpoint's current host and port, for callers that
// render CLI arguments rather than a URL.
func (endpoint *Endpoint) HostPort() (string, string) {
	endpoint.mu.RLock()
	defer endpoint.mu.RUnlock()
	return endpoint.host, endpoint.port
}

// Reserve assigns a free port to a dynamic endpoint. It is a no-op for a
// pinned endpoint. Reservation is sticky: once a dynamic endpoint has a
// live port, later calls only re-reserve if that port is no longer
// available (e.g. taken by an external process between restarts).
func (endpoint *Endpoint) Reserve(allocator *portalloc.Allocator) error {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	if endpoint.pinned != "" {
		return nil
	}
	if endpoint.port != "" && endpoint.port != "0" {
		if err := portalloc.CheckAvailable(endpoint.host, endpoint.port); err == nil {
			return nil
		}
		allocator.Release(endpoint.host, endpoint.port)
		endpoint.port = "0"
	}
	port, err := allocator.Reserve(endpoint.host)
	if err != nil {
		return err
	}
	endpoint.port = port
	return nil
}

// Release returns a dynamic endpoint's port to the allocator and resets the
// endpoint to its unreserved (port "0") state. It is a no-op for a pinned
// endpoint or one that was never reserved.
func (endpoint *Endpoint) Release(allocator *portalloc.Allocator) {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	if endpoint.pinned != "" || endpoint.port == "" || endpoint.port == "0" {
		return
	}
	allocator.Release(endpoint.host, endpoint.port)
	endpoint.port = "0"
}

// CheckAvailable probes whether the endpoint's current port can be bound
// right now. Call it immediately before spawning the backend process so a
// port already held by something else fails fast and by name instead of
// surfacing as a health-check timeout. It is meaningful for both pinned and
// dynamic endpoints: a dynamic endpoint's sticky port can be taken by an
// external process between restarts just like a pinned one.
func (endpoint *Endpoint) CheckAvailable() error {
	host, port := endpoint.HostPort()
	return portalloc.CheckAvailable(host, port)
}
