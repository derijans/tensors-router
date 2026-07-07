package analytics

import (
	"context"
	"database/sql"
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

func TestStoreSeparatesModelLoadsFromRequestCounts(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store.Record(Event{
		ModelID:         "llm-a",
		Section:         SectionLLM,
		BackendMode:     "kobold",
		EventType:       EventTypeModelLoad,
		Route:           "model_load",
		ConfigFilename:  "llm-a.kcpps",
		StatusCode:      200,
		Success:         true,
		StartedAt:       now.Add(-4 * time.Second),
		FinishedAt:      now.Add(-2 * time.Second),
		DurationMS:      2000,
		LoadVRAMBefore:  1024,
		LoadVRAMAfter:   4096,
		VRAMTotal:       8192,
		VRAMPeakPercent: 50,
	})
	store.Record(Event{
		ModelID:         "llm-a",
		Section:         SectionLLM,
		BackendMode:     "kobold",
		Route:           "/v1/chat/*",
		StatusCode:      200,
		Success:         true,
		StartedAt:       now.Add(-time.Second),
		FinishedAt:      now,
		DurationMS:      1000,
		TotalTokens:     8,
		WorkVRAMStart:   4096,
		WorkVRAMMax:     5120,
		WorkVRAMEnd:     4608,
		ModelVRAM:       4096,
		VRAMTotal:       8192,
		VRAMPeakPercent: 62.5,
	})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 1 || response.Summary.LoadCount != 1 {
		t.Fatalf("unexpected request/load counts %#v", response.Summary)
	}
	if response.Summary.AverageLoadMS != 2000 {
		t.Fatalf("unexpected load average %#v", response.Summary)
	}
	if response.Summary.VRAMPeakMB != 5120 || response.Summary.ModelVRAMMB != 4096 || response.Summary.VRAMTotalMB != 8192 || response.Summary.VRAMPeakPercent != 62.5 {
		t.Fatalf("unexpected vram summary %#v", response.Summary)
	}
	if len(response.Models) != 1 || response.Models[0].LoadCount != 1 || response.Models[0].RequestCount != 1 || response.Models[0].VRAMPeakMB != 5120 {
		t.Fatalf("unexpected model usage %#v", response.Models)
	}
	if len(response.Recent) != 2 || response.Recent[1].EventType != EventTypeModelLoad || response.Recent[1].ConfigFilename != "llm-a.kcpps" {
		t.Fatalf("unexpected recent load rows %#v", response.Recent)
	}
}

func TestStoreMigratesOldAnalyticsDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "analytics.sqlite")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE analytics_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		section TEXT NOT NULL,
		backend_mode TEXT NOT NULL,
		route TEXT NOT NULL,
		status_code INTEGER NOT NULL,
		success INTEGER NOT NULL,
		started_at INTEGER NOT NULL,
		finished_at INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		tokens_per_second REAL NOT NULL DEFAULT 0,
		image_count INTEGER NOT NULL DEFAULT 0,
		image_width INTEGER NOT NULL DEFAULT 0,
		image_height INTEGER NOT NULL DEFAULT 0,
		image_type TEXT NOT NULL DEFAULT '',
		audio_seconds REAL NOT NULL DEFAULT 0,
		audio_tokens INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE analytics_rollups (
		period_kind TEXT NOT NULL,
		bucket_start INTEGER NOT NULL,
		node_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		section TEXT NOT NULL,
		backend_mode TEXT NOT NULL,
		route TEXT NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		success_count INTEGER NOT NULL DEFAULT 0,
		duration_ms_total INTEGER NOT NULL DEFAULT 0,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		tokens_per_second_sum REAL NOT NULL DEFAULT 0,
		tokens_per_second_count INTEGER NOT NULL DEFAULT 0,
		image_count INTEGER NOT NULL DEFAULT 0,
		audio_seconds REAL NOT NULL DEFAULT 0,
		audio_tokens INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (period_kind, bucket_start, node_id, model_id, section, backend_mode, route)
	)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(StoreConfig{
		NodeID:        "node-a",
		DatabasePath:  databasePath,
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
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now, WorkVRAMMax: 2048, VRAMTotal: 8192})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 1 || response.Summary.VRAMPeakMB != 2048 {
		t.Fatalf("unexpected migrated response %#v", response.Summary)
	}
}

func TestMergeCombinesVRAMAnalytics(t *testing.T) {
	response := Merge(
		Response{
			Enabled: true,
			Summary: Summary{
				RequestCount:    1,
				SuccessCount:    1,
				LoadCount:       1,
				AverageLoadMS:   1000,
				VRAMPeakMB:      4096,
				VRAMPeakPercent: 50,
				VRAMTotalMB:     8192,
				ModelVRAMMB:     3000,
				AverageDuration: 20,
				AverageTokensPS: 10,
			},
			Timeline: []Timeline{{
				BucketStart:  100,
				RequestCount: 1,
				LoadCount:    1,
				VRAMPeakMB:   4096,
				VRAMPeakPct:  50,
				VRAMTotalMB:  8192,
				ModelVRAMMB:  3000,
			}},
			Sections: []SectionUsage{{Section: SectionLLM, RequestCount: 1, LoadCount: 1, VRAMPeakMB: 4096, VRAMPeakPct: 50, ModelVRAMMB: 3000}},
			Models:   []ModelUsage{{NodeID: "node-a", ModelID: "llm", RequestCount: 1, LoadCount: 1, AverageLoadMS: 1000, VRAMPeakMB: 4096, VRAMPeakPct: 50, ModelVRAMMB: 3000}},
			Nodes:    []NodeUsage{{NodeID: "node-a", RequestCount: 1, LoadCount: 1, AverageLoadMS: 1000, VRAMPeakMB: 4096, VRAMPeakPct: 50, ModelVRAMMB: 3000}},
		},
		Response{
			Enabled: true,
			Summary: Summary{
				RequestCount:    2,
				SuccessCount:    1,
				LoadCount:       3,
				AverageLoadMS:   3000,
				VRAMPeakMB:      6144,
				VRAMPeakPercent: 75,
				VRAMTotalMB:     8192,
				ModelVRAMMB:     5000,
				AverageDuration: 40,
				AverageTokensPS: 20,
			},
			Timeline: []Timeline{{
				BucketStart:  100,
				RequestCount: 2,
				LoadCount:    3,
				VRAMPeakMB:   6144,
				VRAMPeakPct:  75,
				VRAMTotalMB:  8192,
				ModelVRAMMB:  5000,
			}},
			Sections: []SectionUsage{{Section: SectionLLM, RequestCount: 2, LoadCount: 3, VRAMPeakMB: 6144, VRAMPeakPct: 75, ModelVRAMMB: 5000}},
			Models:   []ModelUsage{{NodeID: "node-a", ModelID: "llm", RequestCount: 2, LoadCount: 3, AverageLoadMS: 3000, VRAMPeakMB: 6144, VRAMPeakPct: 75, ModelVRAMMB: 5000}},
			Nodes:    []NodeUsage{{NodeID: "node-a", RequestCount: 2, LoadCount: 3, AverageLoadMS: 3000, VRAMPeakMB: 6144, VRAMPeakPct: 75, ModelVRAMMB: 5000}},
		},
	)

	if response.Summary.RequestCount != 3 || response.Summary.FailureCount != 1 || response.Summary.LoadCount != 4 {
		t.Fatalf("unexpected merged counts %#v", response.Summary)
	}
	if response.Summary.AverageLoadMS != 2500 || response.Summary.VRAMPeakMB != 6144 || response.Summary.VRAMPeakPercent != 75 || response.Summary.ModelVRAMMB != 5000 {
		t.Fatalf("unexpected merged vram summary %#v", response.Summary)
	}
	if len(response.Timeline) != 1 || response.Timeline[0].LoadCount != 4 || response.Timeline[0].VRAMPeakMB != 6144 {
		t.Fatalf("unexpected merged timeline %#v", response.Timeline)
	}
	if len(response.Sections) != 1 || response.Sections[0].LoadCount != 4 || response.Sections[0].ModelVRAMMB != 5000 {
		t.Fatalf("unexpected merged sections %#v", response.Sections)
	}
	if len(response.Models) != 1 || response.Models[0].AverageLoadMS != 2500 || response.Models[0].VRAMPeakMB != 6144 {
		t.Fatalf("unexpected merged models %#v", response.Models)
	}
	if len(response.Nodes) != 1 || response.Nodes[0].AverageLoadMS != 2500 || response.Nodes[0].VRAMPeakMB != 6144 {
		t.Fatalf("unexpected merged nodes %#v", response.Nodes)
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

func TestStoreCloseFlushesBufferedEvents(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "analytics.sqlite")
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store, err := NewStore(StoreConfig{
		NodeID:        "node-a",
		DatabasePath:  databasePath,
		FlushInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})
	if err := store.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(StoreConfig{
		NodeID:        "node-a",
		DatabasePath:  databasePath,
		FlushInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	response, err := reopened.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 1 {
		t.Fatalf("close did not flush buffered event: %#v", response.Summary)
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
