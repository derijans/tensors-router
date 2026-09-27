package analytics

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/routerstore/routerstoretest"
)

func TestQueryReportsWhichRouterBuildRecordedEachNodesEvents(t *testing.T) {
	handle := routerstoretest.Open(t, SchemaModule{})
	store, err := NewStore(StoreConfig{NodeID: "master", RouterVersion: "v0.7.3", DB: handle.DB(), ReadDB: handle.Reader(), FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	now := time.Date(2026, 9, 26, 20, 0, 0, 0, time.UTC)
	store.Record(Event{ModelID: "gemma", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now.Add(time.Second)})
	store.Record(Event{NodeID: "slave", RouterVersion: "v0.7.1", ModelID: "krea", Section: SectionImage, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now.Add(2 * time.Second)})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}

	want := []VersionUsage{
		{NodeID: "master", RouterVersion: "v0.7.3", FirstSeen: now.UnixMilli(), LastSeen: now.Add(time.Second).UnixMilli(), EventCount: 1},
		{NodeID: "slave", RouterVersion: "v0.7.1", FirstSeen: now.UnixMilli(), LastSeen: now.Add(2 * time.Second).UnixMilli(), EventCount: 1},
	}
	if !reflect.DeepEqual(response.Versions, want) {
		t.Fatalf("versions = %+v, want %+v", response.Versions, want)
	}
}

func TestMergeOfNodesWithNoEventsKeepsEveryListEncodableAsEmpty(t *testing.T) {
	merged := Merge(Response{Enabled: true}, Response{Enabled: true})

	encoded, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "null") {
		t.Fatalf("merged response %s, want empty lists instead of null", encoded)
	}
}
