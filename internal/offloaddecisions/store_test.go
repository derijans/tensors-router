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

	records, err := store.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
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

	records, err := store.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].NodeID != "master" || records[0].RecordedAt.IsZero() {
		t.Fatalf("records = %+v, want the store's node and a timestamp", records)
	}
}

func TestFlushPrunesDecisionsOlderThanTheRetention(t *testing.T) {
	store := newTestStore(t, time.Hour)
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeSkipped, RecordedAt: time.Now().Add(-2 * time.Hour)})
	store.Record(Record{Kind: KindPlan, Lane: "image", Outcome: OutcomeGranted})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	records, err := store.Recent(context.Background(), 10)
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

	records, err := store.Recent(context.Background(), 10)
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
