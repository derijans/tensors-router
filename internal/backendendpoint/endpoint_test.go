package backendendpoint

import (
	"strings"
	"sync"
	"testing"

	"tensors-router/internal/portalloc"
)

func TestNewEndpointRejectsMissingPort(t *testing.T) {
	_, err := NewEndpoint("http://127.0.0.1")
	if err == nil {
		t.Fatal("expected an error for a URL with no port")
	}
	if !strings.Contains(err.Error(), "explicit port") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewEndpointRejectsInvalidLoopback(t *testing.T) {
	if _, err := NewEndpoint("http://example.com:5001"); err == nil {
		t.Fatal("expected non-loopback host to be rejected")
	}
}

func TestPinnedEndpointReportsFixedPort(t *testing.T) {
	endpoint, err := NewEndpoint("http://127.0.0.1:6000")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Dynamic() {
		t.Fatal("expected a pinned endpoint")
	}
	host, port := endpoint.HostPort()
	if host != "127.0.0.1" || port != "6000" {
		t.Fatalf("unexpected host/port: %s %s", host, port)
	}
	if got := endpoint.URL().String(); got != "http://127.0.0.1:6000" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestDynamicEndpointURLBeforeReserve(t *testing.T) {
	endpoint, err := NewEndpoint("http://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if !endpoint.Dynamic() {
		t.Fatal("expected a dynamic endpoint")
	}
	_, port := endpoint.HostPort()
	if port != "0" {
		t.Fatalf("expected port 0 before reservation, got %s", port)
	}
}

func TestPinnedEndpointReserveIsNoOp(t *testing.T) {
	endpoint, err := NewEndpoint("http://127.0.0.1:6001")
	if err != nil {
		t.Fatal(err)
	}
	allocator := portalloc.New()
	if err := endpoint.Reserve(allocator); err != nil {
		t.Fatal(err)
	}
	_, port := endpoint.HostPort()
	if port != "6001" {
		t.Fatalf("pinned endpoint changed port after Reserve: %s", port)
	}
	endpoint.Release(allocator) // must not panic or affect the pinned port
	_, port = endpoint.HostPort()
	if port != "6001" {
		t.Fatalf("pinned endpoint changed port after Release: %s", port)
	}
}

func TestDynamicEndpointReserveAssignsDistinctPort(t *testing.T) {
	allocator := portalloc.New()
	first, err := NewEndpoint("http://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEndpoint("http://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Reserve(allocator); err != nil {
		t.Fatal(err)
	}
	if err := second.Reserve(allocator); err != nil {
		t.Fatal(err)
	}
	_, firstPort := first.HostPort()
	_, secondPort := second.HostPort()
	if firstPort == "0" || secondPort == "0" {
		t.Fatalf("expected reserved ports, got %s and %s", firstPort, secondPort)
	}
	if firstPort == secondPort {
		t.Fatalf("expected distinct ports, both got %s", firstPort)
	}
	// Reserve only tracks the port logically; nothing is bound on the OS
	// after it returns, so the port is available to a plain probe.
	if err := first.CheckAvailable(); err != nil {
		t.Fatalf("expected reserved-but-unbound port to be available to a probe: %v", err)
	}
}

func TestDynamicEndpointReserveIsSticky(t *testing.T) {
	allocator := portalloc.New()
	endpoint, err := NewEndpoint("http://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Reserve(allocator); err != nil {
		t.Fatal(err)
	}
	_, first := endpoint.HostPort()
	if err := endpoint.Reserve(allocator); err != nil {
		t.Fatal(err)
	}
	_, second := endpoint.HostPort()
	if first != second {
		t.Fatalf("expected sticky reservation to keep the same port, got %s then %s", first, second)
	}
}

func TestDynamicEndpointReleaseResetsToUnreserved(t *testing.T) {
	allocator := portalloc.New()
	endpoint, err := NewEndpoint("http://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Reserve(allocator); err != nil {
		t.Fatal(err)
	}
	endpoint.Release(allocator)
	_, port := endpoint.HostPort()
	if port != "0" {
		t.Fatalf("expected port to reset to 0 after Release, got %s", port)
	}
}

func TestConcurrentURLAndReserve(t *testing.T) {
	allocator := portalloc.New()
	endpoint, err := NewEndpoint("http://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = endpoint.URL()
			_, _ = endpoint.HostPort()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = endpoint.Reserve(allocator)
			endpoint.Release(allocator)
		}
	}()
	wg.Wait()
}
