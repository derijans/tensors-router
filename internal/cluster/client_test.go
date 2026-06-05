package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientRejectsUnauthorizedTarget(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient("secret")
	err := client.JSON(context.Background(), http.MethodGet, server.URL, "/router/v1/node/models", nil, nil)
	if err == nil {
		t.Fatalf("expected unauthorized target error")
	}
	if hits.Load() != 0 {
		t.Fatalf("unexpected request count %d", hits.Load())
	}
}

func TestClientAllowsAuthorizedTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing cluster authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"node_id":"slave-a","node_url":"` + serverURL(r) + `","models":[]}`))
	}))
	defer server.Close()

	client := NewClient("secret", server.URL)
	var snapshot Snapshot
	if err := client.JSON(context.Background(), http.MethodGet, server.URL, "/router/v1/node/models", nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.NodeID != "slave-a" {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
