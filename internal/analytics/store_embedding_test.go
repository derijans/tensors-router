package analytics

import (
	"context"
	"testing"
	"time"

	"tensors-router/internal/routerstore/routerstoretest"
)

func TestStoreReportsEmbeddingVectorsFromRawEventsAndRollups(t *testing.T) {
	handle := routerstoretest.Open(t, SchemaModule{})
	store, err := NewStore(StoreConfig{
		NodeID:        "node-a",
		DB:            handle.DB(),
		ReadDB:        handle.Reader(),
		FlushInterval: time.Hour,
		RawRetention:  24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	now := time.Now().UTC()
	beyondRetention := now.Add(-72 * time.Hour)
	withinRetention := now.Add(-2 * time.Hour)
	store.Record(Event{ModelID: "embed", Section: SectionEmbed, Route: "/v1/embeddings", StatusCode: 200, Success: true, StartedAt: beyondRetention, FinishedAt: beyondRetention, InputTokens: 40, EmbeddingCount: 150})
	store.Record(Event{ModelID: "embed", Section: SectionEmbed, Route: "/v1/embeddings", StatusCode: 200, Success: true, StartedAt: withinRetention, FinishedAt: withinRetention, InputTokens: 6, EmbeddingCount: 2})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	recentOnly, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-3 * time.Hour).UnixMilli(), EndMS: now.UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if recentOnly.Summary.EmbeddingCount != 2 || recentOnly.Sections[0].EmbeddingCount != 2 || recentOnly.Models[0].EmbeddingCount != 2 || recentOnly.Nodes[0].EmbeddingCount != 2 {
		t.Fatalf("raw event query lost embedding vectors %#v", recentOnly)
	}
	if len(recentOnly.Timeline) != 1 || recentOnly.Timeline[0].EmbeddingCount != 2 {
		t.Fatalf("timeline lost embedding vectors %#v", recentOnly.Timeline)
	}
	if len(recentOnly.Recent) != 1 || recentOnly.Recent[0].EmbeddingCount != 2 {
		t.Fatalf("recent events lost embedding vectors %#v", recentOnly.Recent)
	}

	historical, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: beyondRetention.Add(-time.Hour).UnixMilli(), EndMS: now.UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if historical.Summary.EmbeddingCount != 152 || historical.Sections[0].EmbeddingCount != 152 || historical.Models[0].EmbeddingCount != 152 || historical.Nodes[0].EmbeddingCount != 152 {
		t.Fatalf("rollup query lost embedding vectors %#v", historical)
	}
}
