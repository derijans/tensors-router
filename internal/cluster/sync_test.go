package cluster

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSyncSlavesFetchesConcurrentlyAndAppliesConfiguredOrder(t *testing.T) {
	newSlave := func(delay time.Duration) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(delay)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"node_id":"shared","models":[]}`))
		}))
	}
	first := newSlave(100 * time.Millisecond)
	defer first.Close()
	second := newSlave(100 * time.Millisecond)
	defer second.Close()
	registry := NewRegistry(RoleMaster, "master", "http://master")
	client := NewClient("", first.URL, second.URL)
	acceptedURLs := []string{}
	config := SyncConfig{
		Role:            RoleMaster,
		SlaveURLs:       []string{first.URL, second.URL},
		SyncConcurrency: 2,
		AcceptNodeURL: func(nodeURL string) error {
			if owner := registry.NodeURLsByID()["shared"]; !BaseURLEqual(owner, nodeURL) {
				t.Fatalf("callback ran before registry acceptance: owner=%q callback=%q", owner, nodeURL)
			}
			acceptedURLs = append(acceptedURLs, nodeURL)
			return nil
		},
	}
	started := time.Now()
	SyncConfiguredSlaves(context.Background(), config, registry, client, log.New(io.Discard, "", 0))
	if elapsed := time.Since(started); elapsed > 180*time.Millisecond {
		t.Fatalf("slave fetches were serialized: %s", elapsed)
	}
	if owner := registry.NodeURLsByID()["shared"]; !BaseURLEqual(owner, first.URL) {
		t.Fatalf("configured first owner did not win: %q", owner)
	}
	if len(acceptedURLs) != 1 || !BaseURLEqual(acceptedURLs[0], first.URL) {
		t.Fatalf("authorization callback ran for rejected owner: %#v", acceptedURLs)
	}
}

func TestRegisterInitialAndLoopSurfaceDuplicateNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"type":"cluster_error","code":"duplicate_node","message":"owned"}}`))
	}))
	defer server.Close()
	registry := NewRegistry(RoleSlave, "duplicate", "http://duplicate")
	client := NewClient("", server.URL)
	config := SyncConfig{Role: RoleSlave, MasterURL: server.URL, SyncInterval: 10 * time.Millisecond}
	if err := RegisterInitial(context.Background(), config, registry, client, log.New(io.Discard, "", 0)); remoteErrorCode(err) != ErrorCodeDuplicateNode {
		t.Fatalf("initial duplicate was not terminal: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := StartSync(ctx, config, registry, client, log.New(io.Discard, "", 0))
	select {
	case err := <-errCh:
		if remoteErrorCode(err) != ErrorCodeDuplicateNode {
			t.Fatalf("unexpected terminal error %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("registration loop did not surface duplicate")
	}
}

func TestRegistrationLoopRetriesTransientFailure(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	registry := NewRegistry(RoleSlave, "slave", "http://slave")
	client := NewClient("", server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := StartSync(ctx, SyncConfig{Role: RoleSlave, MasterURL: server.URL, SyncInterval: 10 * time.Millisecond}, registry, client, log.New(io.Discard, "", 0))
	deadline := time.After(time.Second)
	for requests.Load() < 2 {
		select {
		case err := <-errCh:
			t.Fatalf("transient registration became terminal: %v", err)
		case <-deadline:
			t.Fatal("transient registration was not retried")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func remoteErrorCode(err error) string {
	if remoteError, ok := err.(*RemoteError); ok {
		return remoteError.Code
	}
	return ""
}

func TestSyncSlavesCancelsOutstandingProbeOnShutdown(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	registry := NewRegistry(RoleMaster, "master", "http://master")
	client := NewClientWithTimeout("", 10*time.Second, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		SyncConfiguredSlaves(ctx, SyncConfig{Role: RoleMaster, SlaveURLs: []string{server.URL}, SyncConcurrency: 1}, registry, client, log.New(io.Discard, "", 0))
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the outstanding cluster probe")
	}
}
func TestTerminalRegistrationErrorRequiresConflictClusterEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		terminal bool
	}{
		{name: "duplicate conflict", err: &RemoteError{StatusCode: http.StatusConflict, Type: "cluster_error", Code: ErrorCodeDuplicateNode}, terminal: true},
		{name: "wrong status", err: &RemoteError{StatusCode: http.StatusInternalServerError, Type: "cluster_error", Code: ErrorCodeDuplicateNode}},
		{name: "wrong type", err: &RemoteError{StatusCode: http.StatusConflict, Type: "upstream_error", Code: ErrorCodeDuplicateNode}},
		{name: "wrong code", err: &RemoteError{StatusCode: http.StatusConflict, Type: "cluster_error", Code: "busy"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if terminal := terminalRegistrationError(test.err); terminal != test.terminal {
				t.Fatalf("terminal=%t want %t for %v", terminal, test.terminal, test.err)
			}
		})
	}
}
