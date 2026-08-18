// Package portalloc allocates free loopback TCP ports for locally spawned
// backend processes. It exists because the router's backend managers pick a
// port for a child process before that child ever binds it: the actual bind
// happens inside the spawned process (koboldcpp, llama-server, sd-server,
// whisper-server), none of which accept an inherited socket. Reserve/Release
// close the window between two managers *inside this router* racing for the
// same ephemeral port; they cannot close the window against an unrelated
// external process binding the port between Reserve and the child's own
// bind() call. See CheckAvailable for a fast, synchronous probe of that
// residual case.
package portalloc

import (
	"fmt"
	"net"
	"sync"
)

const maxReserveAttempts = 32

// Allocator hands out loopback TCP ports that are not currently reserved by
// this process. It is safe for concurrent use.
type Allocator struct {
	mu       sync.Mutex
	reserved map[string]struct{}
	listen   func(network, address string) (net.Listener, error)
}

// New returns an Allocator. Most callers should use Default.
func New() *Allocator {
	return &Allocator{
		reserved: make(map[string]struct{}),
		listen:   net.Listen,
	}
}

var defaultAllocator = New()

// Default returns the process-wide allocator shared by all backend managers.
func Default() *Allocator {
	return defaultAllocator
}

// Reserve picks a free TCP port on host and marks it reserved until Release
// is called. It briefly binds and immediately closes a listener to learn a
// free port from the kernel, then re-checks the result against this
// allocator's own reserved set so two concurrent Reserve calls in this
// process never return the same port even if the kernel handed out the same
// ephemeral port twice in a row (observed in practice on some platforms
// under load).
func (allocator *Allocator) Reserve(host string) (string, error) {
	for attempt := 0; attempt < maxReserveAttempts; attempt++ {
		port, err := allocator.probeFreePort(host)
		if err != nil {
			return "", err
		}
		allocator.mu.Lock()
		key := net.JoinHostPort(host, port)
		if _, taken := allocator.reserved[key]; taken {
			allocator.mu.Unlock()
			continue
		}
		allocator.reserved[key] = struct{}{}
		allocator.mu.Unlock()
		return port, nil
	}
	return "", fmt.Errorf("portalloc: no free loopback port on %s after %d attempts", host, maxReserveAttempts)
}

func (allocator *Allocator) probeFreePort(host string) (string, error) {
	listener, err := allocator.listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return "", fmt.Errorf("portalloc: could not probe a free port on %s: %w", host, err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("portalloc: could not read probed port on %s: %w", host, err)
	}
	return port, nil
}

// Release returns a previously reserved host:port to the pool. Releasing a
// port that was never reserved (or already released) is a no-op.
func (allocator *Allocator) Release(host, port string) {
	if port == "" {
		return
	}
	allocator.mu.Lock()
	delete(allocator.reserved, net.JoinHostPort(host, port))
	allocator.mu.Unlock()
}

// CheckAvailable reports whether host:port can be bound right now. It is a
// point-in-time probe, not a reservation: use it immediately before spawning
// a child process to turn a silent bind failure into a named, immediate
// error instead of a health-check timeout.
func CheckAvailable(host, port string) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("backend port %s on %s is already in use: %w", port, host, err)
	}
	return listener.Close()
}
