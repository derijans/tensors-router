package analytics

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStoreFlushesAndQueriesAnalytics(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Date(2026, 6, 25, 12, 30, 0, 0, time.UTC)
	store.Record(Event{
		ModelID:         "llm-a",
		Section:         SectionLLM,
		BackendMode:     "kobold",
		Route:           "/v1/chat/*",
		StatusCode:      200,
		Success:         true,
		StartedAt:       now.Add(-2 * time.Second),
		FinishedAt:      now,
		DurationMS:      2000,
		InputTokens:     10,
		OutputTokens:    5,
		TokensPerSecond: 2.5,
	})
	store.Record(Event{
		ModelID:     "image-a",
		Section:     SectionImage,
		BackendMode: "llama_sdcpp",
		Route:       "/sdapi/v1/*",
		StatusCode:  500,
		Success:     false,
		StartedAt:   now.Add(-time.Second),
		FinishedAt:  now.Add(time.Second),
		DurationMS:  1000,
		ImageCount:  2,
		ImageWidth:  512,
		ImageHeight: 768,
		ImageType:   "txt2img",
	})

	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	response, err := store.Query(context.Background(), Query{
		Period:  Period24Hours,
		StartMS: now.Add(-time.Hour).UnixMilli(),
		EndMS:   now.Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !response.Enabled {
		t.Fatalf("expected enabled response")
	}
	if response.Summary.RequestCount != 2 || response.Summary.SuccessCount != 1 || response.Summary.FailureCount != 1 {
		t.Fatalf("unexpected summary counts %#v", response.Summary)
	}
	if response.Summary.InputTokens != 10 || response.Summary.OutputTokens != 5 || response.Summary.TotalTokens != 15 {
		t.Fatalf("unexpected token summary %#v", response.Summary)
	}
	if response.Summary.ImageCount != 2 {
		t.Fatalf("unexpected image summary %#v", response.Summary)
	}
	if len(response.Timeline) != 1 || response.Timeline[0].RequestCount != 2 {
		t.Fatalf("unexpected timeline %#v", response.Timeline)
	}
	if len(response.Recent) != 2 || response.Recent[0].ModelID != "image-a" {
		t.Fatalf("unexpected recent rows %#v", response.Recent)
	}
}

func TestStoreFiltersByNodeModelAndSection(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store.Record(Event{NodeID: "node-a", ModelID: "same", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})
	store.Record(Event{NodeID: "node-b", ModelID: "same", Section: SectionImage, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now, ImageCount: 1})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := store.Query(context.Background(), Query{
		Period:  PeriodAll,
		StartMS: now.Add(-time.Hour).UnixMilli(),
		EndMS:   now.Add(time.Hour).UnixMilli(),
		NodeID:  "node-b",
		ModelID: "same",
		Section: SectionImage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 1 || response.Summary.ImageCount != 1 {
		t.Fatalf("unexpected filtered response %#v", response.Summary)
	}
}

func TestStoreHandlesConcurrentRecorders(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})
		}()
	}
	wait.Wait()
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 20 {
		t.Fatalf("unexpected concurrent count %d", response.Summary.RequestCount)
	}
}

func TestStoreDoesNotPersistContentFields(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, Route: "/v1/chat/*", StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now, TotalTokens: 4})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Recent) != 1 || response.Recent[0].TotalTokens != 4 {
		t.Fatalf("unexpected recent response %#v", response.Recent)
	}
}

func newTestStore(t *testing.T, nodeID string) *Store {
	t.Helper()
	store, err := NewStore(StoreConfig{
		NodeID:        nodeID,
		DatabasePath:  filepath.Join(t.TempDir(), "analytics.sqlite"),
		FlushInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	return store
}
