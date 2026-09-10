package analytics

import (
	"context"
	"testing"
	"time"
)

func TestQueryPeriodNarrowsTheWindow(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	for _, age := range []time.Duration{time.Hour, 48 * time.Hour, 10 * 24 * time.Hour, 60 * 24 * time.Hour} {
		finished := now.Add(-age)
		store.Record(Event{
			ModelID:      "llm-a",
			Section:      SectionLLM,
			BackendMode:  "kobold",
			Route:        "/v1/chat/*",
			StatusCode:   200,
			Success:      true,
			StartedAt:    finished.Add(-time.Second),
			FinishedAt:   finished,
			DurationMS:   1000,
			InputTokens:  10,
			OutputTokens: 5,
		})
	}
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	expected := map[string]int64{
		Period24Hours: 1,
		Period7Days:   2,
		Period30Days:  3,
		Period90Days:  4,
		PeriodAll:     4,
	}
	for period, want := range expected {
		response, err := store.Query(context.Background(), Query{Period: period})
		if err != nil {
			t.Fatalf("period %s: %v", period, err)
		}
		if response.Summary.RequestCount != want {
			t.Errorf("period %s: request count = %d, want %d", period, response.Summary.RequestCount, want)
		}
	}
}

func TestQueryPeriodNarrowsRecentEvents(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	for _, age := range []time.Duration{time.Hour, 10 * 24 * time.Hour} {
		finished := now.Add(-age)
		store.Record(Event{
			ModelID:     "llm-a",
			Section:     SectionLLM,
			BackendMode: "kobold",
			StatusCode:  200,
			Success:     true,
			StartedAt:   finished.Add(-time.Second),
			FinishedAt:  finished,
			DurationMS:  1000,
		})
	}
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	recent, err := store.Query(context.Background(), Query{Period: Period24Hours})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Recent) != 1 {
		t.Fatalf("24h recent = %d events, want 1", len(recent.Recent))
	}
	all, err := store.Query(context.Background(), Query{Period: PeriodAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Recent) != 2 {
		t.Fatalf("all recent = %d events, want 2", len(all.Recent))
	}
}
