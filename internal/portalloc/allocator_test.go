package portalloc

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

func TestReserveReturnsDistinctPortsConcurrently(t *testing.T) {
	allocator := New()
	const workers = 64
	ports := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(index int) {
			defer wg.Done()
			port, err := allocator.Reserve("127.0.0.1")
			ports[index] = port
			errs[index] = err
		}(i)
	}
	wg.Wait()

	seen := make(map[string]struct{}, workers)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: unexpected error: %v", i, err)
		}
		if ports[i] == "" {
			t.Fatalf("worker %d: empty port", i)
		}
		if _, dup := seen[ports[i]]; dup {
			t.Fatalf("worker %d: duplicate port %s", i, ports[i])
		}
		seen[ports[i]] = struct{}{}
	}
}

func TestReleaseMakesPortReusable(t *testing.T) {
	allocator := New()
	port, err := allocator.Reserve("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	allocator.Release("127.0.0.1", port)

	allocator.mu.Lock()
	_, stillReserved := allocator.reserved[net.JoinHostPort("127.0.0.1", port)]
	allocator.mu.Unlock()
	if stillReserved {
		t.Fatalf("port %s remained reserved after Release", port)
	}
}

func TestReleaseUnknownPortIsNoOp(t *testing.T) {
	allocator := New()
	allocator.Release("127.0.0.1", "59999")
	allocator.Release("127.0.0.1", "")
}

func TestReserveFailsAfterMaxAttempts(t *testing.T) {
	// Learn a real free port, then stub the listener to keep handing back
	// that exact port on every probe. Pre-marking it reserved forces every
	// attempt inside Reserve to collide, so Reserve must give up instead of
	// looping forever.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, fixedPort, err := net.SplitHostPort(probe.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	probe.Close()

	allocator := New()
	allocator.listen = func(network, address string) (net.Listener, error) {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			return nil, splitErr
		}
		return net.Listen(network, net.JoinHostPort(host, fixedPort))
	}
	allocator.mu.Lock()
	allocator.reserved[net.JoinHostPort("127.0.0.1", fixedPort)] = struct{}{}
	allocator.mu.Unlock()

	if _, err := allocator.Reserve("127.0.0.1"); err == nil {
		t.Fatal("expected Reserve to fail once every probed port is already reserved")
	}
}

func TestCheckAvailableDetectsHeldPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckAvailable("127.0.0.1", port); err == nil {
		t.Fatal("expected CheckAvailable to report the held port as unavailable")
	}
}

func TestCheckAvailableAcceptsFreePort(t *testing.T) {
	allocator := New()
	port, err := allocator.Reserve("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	allocator.Release("127.0.0.1", port)
	if err := CheckAvailable("127.0.0.1", port); err != nil {
		t.Fatalf("expected free port to be available: %v", err)
	}
}

func TestReserveErrorWrapsListenFailure(t *testing.T) {
	allocator := New()
	boom := fmt.Errorf("boom")
	allocator.listen = func(string, string) (net.Listener, error) {
		return nil, boom
	}
	if _, err := allocator.Reserve("127.0.0.1"); err == nil {
		t.Fatal("expected error from failing listener")
	}
}
