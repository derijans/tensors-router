package offloaddecisions

import (
	"context"
	"reflect"
	"testing"
	"time"

	"tensors-router/internal/routerstore/routerstoretest"
)

func newTestStore(t *testing.T, retention time.Duration) *Store {
	t.Helper()
	handle := routerstoretest.Open(t, SchemaModule{})
	store, err := NewStore(StoreConfig{NodeID: "master", DB: handle.DB(), ReadDB: handle.Reader(), Retention: retention, FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRecordedDecisionReadsBackWithEveryField(t *testing.T) {
	store := newTestStore(t, 0)
	recordedAt := time.UnixMilli(time.Now().UnixMilli())
	written := Record{
		RecordedAt:    recordedAt,
		NodeID:        "master",
		Kind:          KindPlan,
		Trigger:       "completed",
		Lane:          "image",
		OwnerNodeID:   "slave",
		OwnerModelID:  "krea",
		HelperNodeID:  "master",
		HelperModelID: "krea11",
		Outcome:       OutcomeGranted,
		Reason:        "cost",
		PendingCount:  8,
		BacklogCount:  10,
		KeepMS:        280000,
		SwitchMS:      12000,
		ServiceMS:     11500,
		OwnerJobMS:    28000,
		ServiceSource: ServiceSourceOwnerFallback,
		HelperIdleMS:  6000,
		Slots:         2,
		LentOut:       1,
		BorrowedAhead: 2,
		WaitMS:        900,
	}
	store.Record(written)
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	written.ID = 1
	if !reflect.DeepEqual(records, []Record{written}) {
		t.Fatalf("records = %+v, want %+v", records, []Record{written})
	}
}

func TestRecordFillsNodeAndTimeWhenTheCallerLeavesThemOut(t *testing.T) {
	store := newTestStore(t, 0)
	store.Record(Record{Kind: KindDispatch, Lane: "text", Outcome: OutcomeLent})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].NodeID != "master" || records[0].RecordedAt.IsZero() {
		t.Fatalf("records = %+v, want the store's node and a timestamp", records)
	}
}

func TestRecordIsStampedWithTheRouterVersionThatWroteIt(t *testing.T) {
	handle := routerstoretest.Open(t, SchemaModule{})
	store, err := NewStore(StoreConfig{NodeID: "master", RouterVersion: "v0.7.3", DB: handle.DB(), ReadDB: handle.Reader(), FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeProbe})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].RouterVersion != "v0.7.3" {
		t.Fatalf("records = %+v, want the writer's router version", records)
	}
}

func TestQueryFiltersByLaneOutcomeAndTime(t *testing.T) {
	store := newTestStore(t, 0)
	now := time.Now()
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeProbe, RecordedAt: now.Add(-time.Hour)})
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeProbe, RecordedAt: now})
	store.Record(Record{Kind: KindPlan, Lane: "text", Outcome: OutcomeProbe, RecordedAt: now})
	store.Record(Record{Kind: KindDispatch, Lane: "image", Outcome: OutcomeLent, RecordedAt: now})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), Filter{Since: now.Add(-time.Minute), Lane: "image", Outcome: OutcomeProbe})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Lane != "image" || records[0].Outcome != OutcomeProbe || records[0].RecordedAt.Before(now.Add(-time.Minute)) {
		t.Fatalf("records = %+v, want only the recent image probe", records)
	}
}

func TestQueryOfAnEmptyLogIsAnEmptyList(t *testing.T) {
	records, err := newTestStore(t, 0).Query(context.Background(), Filter{})
	if err != nil || records == nil || len(records) != 0 {
		t.Fatalf("records = %#v, err = %v, want a non-nil empty list", records, err)
	}
}

func TestShortenedRetentionPrunesOnTheNextFlush(t *testing.T) {
	store := newTestStore(t, 24*time.Hour)
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeSkipped, RecordedAt: time.Now().Add(-2 * time.Hour)})

	store.SetRetention(time.Hour)
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	if records, _ := store.Query(context.Background(), Filter{}); len(records) != 0 {
		t.Fatalf("records = %+v, want the decision pruned under the shortened retention", records)
	}
}

func TestFlushPrunesDecisionsOlderThanTheRetention(t *testing.T) {
	store := newTestStore(t, time.Hour)
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeSkipped, RecordedAt: time.Now().Add(-2 * time.Hour)})
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeGranted})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Outcome != OutcomeGranted {
		t.Fatalf("records = %+v, want only the decision inside the retention window", records)
	}
}

func TestCloseWritesWhatIsStillBuffered(t *testing.T) {
	handle := routerstoretest.Open(t, SchemaModule{})
	store, err := NewStore(StoreConfig{NodeID: "slave", DB: handle.DB(), ReadDB: handle.Reader(), FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	store.Record(Record{Kind: KindHelper, Lane: "image", Outcome: OutcomeBorrowedReturned})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v, want the buffered decision written on close", records)
	}
}

func TestNilStoreIgnoresEverything(t *testing.T) {
	var store *Store
	store.Record(Record{Outcome: OutcomeLent})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
