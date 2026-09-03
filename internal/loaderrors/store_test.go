package loaderrors

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T, retention time.Duration) *Store {
	t.Helper()
	store, err := NewStore(StoreConfig{NodeID: "node-a", DatabasePath: filepath.Join(t.TempDir(), "load-errors.sqlite"), Retention: retention})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRecordDeduplicatesByFingerprintAndCountsOccurrences(t *testing.T) {
	store := newTestStore(t, 0)
	ctx := context.Background()
	for attempt := 0; attempt < 3; attempt++ {
		input := RecordInput{
			Phase:      PhaseHealthWait,
			Source:     "native.waitHealthy",
			ConfigName: "llama-8b",
			Message:    "native server did not become healthy within 120s",
			Output:     "llama_load: failed to open model",
		}
		if err := store.Record(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected a single deduplicated record, got %d", len(result.Records))
	}
	if result.Records[0].Occurrences != 3 {
		t.Fatalf("expected occurrences 3, got %d", result.Records[0].Occurrences)
	}
	if result.Records[0].NodeID != "node-a" {
		t.Fatalf("record did not inherit the store node id: %q", result.Records[0].NodeID)
	}
}

func TestRecordFingerprintIgnoresVolatileNumbers(t *testing.T) {
	store := newTestStore(t, 0)
	ctx := context.Background()
	if err := store.Record(ctx, RecordInput{Phase: PhasePortBind, Source: "native", Message: "port 15001 already in use"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(ctx, RecordInput{Phase: PhasePortBind, Source: "native", Message: "port 15002 already in use"}); err != nil {
		t.Fatal(err)
	}
	result, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 || result.Records[0].Occurrences != 2 {
		t.Fatalf("volatile numbers were not normalized away: %#v", result.Records)
	}
}

func TestRecordRedactsSecretsAndTokens(t *testing.T) {
	store := newTestStore(t, 0)
	ctx := context.Background()
	err := store.Record(ctx, RecordInput{
		Phase:   PhaseDownload,
		Message: "auth failed for Authorization: Bearer hf_supersecrettoken",
		Output:  "using key sk-abcdef and password hunter2",
		Secrets: []string{"hunter2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	record := result.Records[0]
	if strings.Contains(record.Message, "hf_supersecrettoken") || strings.Contains(record.Output, "sk-abcdef") || strings.Contains(record.Output, "hunter2") {
		t.Fatalf("secret material survived redaction: %#v", record)
	}
}

func TestRecordPrunesExpiredRows(t *testing.T) {
	store := newTestStore(t, time.Hour)
	store.pruneEach = 0
	ctx := context.Background()
	if err := store.Record(ctx, RecordInput{Phase: PhasePreload, Source: "old", Message: "stale failure"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE load_errors SET last_seen_at = ?`, time.Now().Add(-2*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(ctx, RecordInput{Phase: PhasePreload, Source: "new", Message: "fresh failure"}); err != nil {
		t.Fatal(err)
	}
	result, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 || result.Records[0].Source != "new" {
		t.Fatalf("retention pruning did not drop the stale row: %#v", result.Records)
	}
}

func TestNilStoreRecordIsNoOp(t *testing.T) {
	var store *Store
	if err := store.Record(context.Background(), RecordInput{Phase: PhasePreload, Message: "ignored"}); err != nil {
		t.Fatalf("nil store must be a silent no-op: %v", err)
	}
}
