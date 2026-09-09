package main

import (
	"errors"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/routerstore/routerstoretest"
)

// The shared handle has to outlive every store that writes through it, so the
// analytics flush on shutdown still has a database to land in.
func TestAnalyticsShutdownFlushLandsBeforeTheDatabaseCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.sqlite")
	handle := routerstoretest.OpenAt(t, path, routeranalytics.SchemaModule{})
	discard := log.New(io.Discard, "", 0)
	store, err := routeranalytics.NewStore(routeranalytics.StoreConfig{
		NodeID:        "node-a",
		DB:            handle.DB(),
		ReadDB:        handle.Reader(),
		FlushInterval: time.Hour,
		Logger:        discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store.Record(routeranalytics.Event{ModelID: "llm-a", Section: routeranalytics.SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})

	if err := errors.Join(closeRouterRuntime(nil, nil, store, nil, discard), handle.Close()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	reopened := routerstoretest.OpenAt(t, path, routeranalytics.SchemaModule{})
	var persisted int
	if err := reopened.Reader().QueryRow(`SELECT COUNT(*) FROM analytics_events`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != 1 {
		t.Fatalf("shutdown persisted %d events, want 1", persisted)
	}
}
