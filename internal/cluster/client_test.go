package cluster

import (
	"context"
	"encoding/json"
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

func TestClientForwardsLoadModelAndUnloadTarget(t *testing.T) {
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for key, value := range body {
			seen[r.URL.Path+":"+key] = value
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient("secret", server.URL)
	if err := client.Load(context.Background(), server.URL, "combo"); err != nil {
		t.Fatal(err)
	}
	if err := client.Unload(context.Background(), server.URL, "", "image"); err != nil {
		t.Fatal(err)
	}
	if seen["/router/v1/load:model"] != "combo" {
		t.Fatalf("unexpected load payload %#v", seen)
	}
	if _, ok := seen["/router/v1/load:unload_policy"]; ok {
		t.Fatalf("load should not forward unload policy %#v", seen)
	}
	if seen["/router/v1/unload:target"] != "image" {
		t.Fatalf("unexpected unload payload %#v", seen)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
