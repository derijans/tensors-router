package analytics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyRequestExtractsStepsFromEverySpelling(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
	}{
		{"a1111 and sdcpp", `{"width":1024,"height":1024,"steps":30}`},
		{"diffusers", `{"width":1024,"height":1024,"num_inference_steps":30}`},
		{"sdcpp cli", `{"width":1024,"height":1024,"sample_steps":30}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			event := Event{Section: SectionImage}
			ApplyRequest(&event, "/sdapi/v1/txt2img", []byte(testCase.body), "application/json")
			if event.ImageSteps != 30 {
				t.Fatalf("steps = %d, want 30", event.ImageSteps)
			}
		})
	}
}

func TestApplyRequestLeavesStepsUnsetWhenAbsent(t *testing.T) {
	event := Event{Section: SectionImage}
	ApplyRequest(&event, "/sdapi/v1/txt2img", []byte(`{"width":512,"height":512}`), "application/json")
	if event.ImageSteps != 0 {
		t.Fatalf("steps = %d, want 0 for a request that did not specify any", event.ImageSteps)
	}
}

func TestApplyRequestDoesNotReadStepsForNonImageSections(t *testing.T) {
	event := Event{Section: SectionLLM}
	ApplyRequest(&event, "/v1/chat/completions", []byte(`{"steps":30}`), "application/json")
	if event.ImageSteps != 0 {
		t.Fatalf("steps = %d, want 0 outside the image section", event.ImageSteps)
	}
}

func TestStoreRoundTripsImageSteps(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC().Truncate(time.Second)
	store.Record(Event{
		ModelID:     "sdxl",
		Section:     SectionImage,
		BackendMode: "llama_sdcpp",
		Route:       "/sdapi/v1/*",
		StatusCode:  200,
		Success:     true,
		StartedAt:   now.Add(-8 * time.Second),
		FinishedAt:  now,
		DurationMS:  8000,
		ImageCount:  1,
		ImageWidth:  1024,
		ImageHeight: 1024,
		ImageSteps:  30,
	})

	response, err := store.Query(context.Background(), mustNormalizeQuery(t, Query{Period: Period24Hours}, now))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Recent) != 1 {
		t.Fatalf("recent events = %d, want 1", len(response.Recent))
	}
	if response.Recent[0].ImageSteps != 30 {
		t.Fatalf("stored steps = %d, want 30", response.Recent[0].ImageSteps)
	}
}

// Nodes upgrading from an earlier release already hold a schema without the
// column, so the additive migration has to add it in place rather than requiring
// the database to be rebuilt.
func TestMigrationAddsImageStepsToAnExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.sqlite")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE analytics_events (
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
		image_width INTEGER NOT NULL DEFAULT 0,
		image_height INTEGER NOT NULL DEFAULT 0
	); PRAGMA user_version = 4;`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO analytics_events
		(node_id, model_id, section, backend_mode, route, status_code, success, started_at, finished_at, duration_ms, image_width, image_height)
		VALUES ('node-a','sdxl','image','llama_sdcpp','/sdapi/v1/*',200,1,1,2,8000,1024,1024)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(StoreConfig{NodeID: "node-a", DatabasePath: path, FlushInterval: time.Hour})
	if err != nil {
		t.Fatalf("migration of an existing database failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	var steps sql.NullInt64
	if err := store.db.QueryRow(`SELECT image_steps FROM analytics_events WHERE model_id = 'sdxl'`).Scan(&steps); err != nil {
		t.Fatalf("image_steps was not added to the existing table: %v", err)
	}
	if !steps.Valid || steps.Int64 != 0 {
		t.Fatalf("pre-existing row has steps %v, want 0", steps)
	}

	var version int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("user_version = %d, want 5", version)
	}
}

func mustNormalizeQuery(t *testing.T, query Query, now time.Time) Query {
	t.Helper()
	normalized, err := NormalizeQuery(query, now)
	if err != nil {
		t.Fatal(err)
	}
	return normalized
}
