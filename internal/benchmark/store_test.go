package benchmark

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStoreSavesLatestAndOptionDiffs(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := []Summary{{
		RunID:      "run-1",
		Type:       TypeSection,
		Section:    SectionLLM,
		Status:     StatusSuccess,
		StartedAt:  10,
		FinishedAt: 20,
		DurationMS: 10,
	}}
	record, err := store.SaveRun("node-a", "model-a", TypeSection, first, rawOptions(t, map[string]string{"threads": "4"}))
	if err != nil {
		t.Fatal(err)
	}
	if record.Latest == nil || record.Latest.Status != StatusSuccess {
		t.Fatalf("unexpected first latest %#v", record.Latest)
	}
	if len(record.History) != 1 || len(record.History[0].OptionChanges) != 0 {
		t.Fatalf("first run should not diff empty baseline %#v", record.History)
	}

	second := []Summary{{
		RunID:      "run-2",
		Type:       TypeSection,
		Section:    SectionLLM,
		Status:     StatusSuccess,
		StartedAt:  30,
		FinishedAt: 50,
		DurationMS: 20,
	}}
	record, err = store.SaveRun("node-a", "model-a", TypeSection, second, rawOptions(t, map[string]string{"threads": "8", "batchsize": "512"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(record.History) != 2 {
		t.Fatalf("unexpected history %#v", record.History)
	}
	changes := record.History[1].OptionChanges
	if len(changes) != 2 || changes[0].Key != "batchsize" || changes[0].Kind != "added" || changes[1].Key != "threads" || changes[1].Kind != "changed" {
		t.Fatalf("unexpected changes %#v", changes)
	}
	if string(changes[1].Previous) != `"4"` || string(changes[1].Current) != `"8"` {
		t.Fatalf("unexpected change values %#v", changes[1])
	}
}

func TestStoreBulkLookupUsesPublishedSnapshot(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRun("node-a", "model-a", TypeSection, []Summary{summaryForTest(1)}, nil); err != nil {
		t.Fatal(err)
	}
	keys := []ModelKey{{NodeID: "node-a", ModelID: "model-a"}, {NodeID: "node-a", ModelID: "missing"}}
	benchmarks := store.ModelBenchmarks(keys)
	benchmark, ok := benchmarks[keys[0]]
	if !ok || benchmark.Latest == nil {
		t.Fatalf("bulk lookup missed record %#v", benchmarks)
	}
	benchmark.Latest.Status = StatusFailed
	reloaded := store.ModelBenchmarks(keys)[keys[0]]
	if reloaded.Latest == nil || reloaded.Latest.Status != StatusSuccess {
		t.Fatalf("bulk result mutated snapshot %#v", reloaded)
	}
}

func TestStorePublishesOnlyAfterPersistence(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRun("node-a", "model-a", TypeSection, []Summary{summaryForTest(1)}, nil); err != nil {
		t.Fatal(err)
	}
	store.persist = func(storeFile) error { return errors.New("write failed") }
	if _, err := store.SaveRun("node-a", "model-a", TypeSection, []Summary{summaryForTest(2)}, nil); err == nil {
		t.Fatal("expected persistence failure")
	}
	record, ok, err := store.Record("node-a", "model-a")
	if err != nil || !ok {
		t.Fatalf("record missing ok=%v err=%v", ok, err)
	}
	if len(record.History) != 1 {
		t.Fatalf("failed write changed published snapshot %#v", record)
	}
}

func TestStoreKeepsHistoryLimit(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < HistoryLimit+4; index++ {
		_, err := store.SaveRun("node-a", "model-a", TypeSection, []Summary{{
			RunID:      "run",
			Type:       TypeSection,
			Section:    SectionRuntime,
			Status:     StatusSuccess,
			StartedAt:  int64(index),
			FinishedAt: int64(index + 1),
		}}, rawOptions(t, map[string]string{"threads": "4"}))
		if err != nil {
			t.Fatal(err)
		}
	}
	record, ok, err := store.Record("node-a", "model-a")
	if err != nil || !ok {
		t.Fatalf("record missing ok=%t err=%v", ok, err)
	}
	if len(record.History) != HistoryLimit {
		t.Fatalf("expected capped history, got %d", len(record.History))
	}
}

func TestStoreOptionDiffIgnoresJSONWhitespace(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveRun("node-a", "model-a", TypeSection, []Summary{summaryForTest(1)}, map[string]json.RawMessage{
		"samplers": json.RawMessage(`["top_k", "top_p"]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.SaveRun("node-a", "model-a", TypeSection, []Summary{summaryForTest(2)}, map[string]json.RawMessage{
		"samplers": json.RawMessage(`[
			"top_k",
			"top_p"
		]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.History[1].OptionChanges) != 0 {
		t.Fatalf("unexpected option changes %#v", record.History[1].OptionChanges)
	}
}

func summaryForTest(run int) Summary {
	return Summary{
		RunID:      "run",
		Type:       TypeSection,
		Section:    SectionRuntime,
		Status:     StatusSuccess,
		StartedAt:  int64(run),
		FinishedAt: int64(run + 1),
	}
}

func rawOptions(t *testing.T, values map[string]string) map[string]json.RawMessage {
	t.Helper()
	options := map[string]json.RawMessage{}
	for key, value := range values {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		options[key] = body
	}
	return options
}
