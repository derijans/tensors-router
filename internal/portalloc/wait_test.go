package portalloc

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestWaitForReleaseReturnsOnceALateListenerCloses(t *testing.T) {
	held, port := holdLoopbackPort(t)
	time.AfterFunc(300*time.Millisecond, func() { _ = held.Close() })

	if err := WaitForRelease(context.Background(), "127.0.0.1", port, 5*time.Second); err != nil {
		t.Fatalf("expected the port to become available after the listener closed: %v", err)
	}
}

func TestWaitForReleaseGivesUpAfterTheTimeout(t *testing.T) {
	held, port := holdLoopbackPort(t)
	defer held.Close()

	started := time.Now()
	err := WaitForRelease(context.Background(), "127.0.0.1", port, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected a held port to stay unavailable")
	}
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("expected to give up after the 200ms timeout, took %s", elapsed)
	}
}

func TestWaitForReleaseStopsWhenTheContextEnds(t *testing.T) {
	held, port := holdLoopbackPort(t)
	defer held.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	if err := WaitForRelease(ctx, "127.0.0.1", port, time.Minute); err == nil {
		t.Fatal("expected a held port to stay unavailable")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("expected the ended context to stop the wait, took %s", elapsed)
	}
}

func holdLoopbackPort(t *testing.T) (net.Listener, string) {
	t.Helper()
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(held.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return held, port
}
