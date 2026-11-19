package memorysamples

import (
	"context"
	"testing"

	"tensors-router/internal/memoryfit"
	"tensors-router/internal/routerstore/routerstoretest"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	handle := routerstoretest.Open(t, SchemaModule{})
	store, err := NewStore(handle.DB(), handle.Reader())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func combo(contextSize int) memoryfit.Combo {
	return memoryfit.Combo{NodeID: "node-a", WeightsHash: "hash-a", BackendMode: "kobold", GPULayers: 99, ContextSize: contextSize}
}

func TestRecordKeepsOnlyTheNewestTwentySamplesPerCombo(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for index := range 25 {
		if err := store.Record(ctx, Record{Combo: combo(8192), VRAMMB: int64(1000 + index)}); err != nil {
			t.Fatal(err)
		}
	}
	input, err := store.EstimateInput(ctx, combo(8192), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Exact) != SamplesPerCombo {
		t.Fatalf("kept %d samples, want %d", len(input.Exact), SamplesPerCombo)
	}
	for _, sample := range input.Exact {
		if sample.VRAMMB < 1005 {
			t.Fatalf("an old sample survived the trim: %+v", sample)
		}
	}
}

func TestTrimmingOneComboLeavesOthersAlone(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Record(ctx, Record{Combo: combo(4096), VRAMMB: 5000}); err != nil {
		t.Fatal(err)
	}
	for range 25 {
		if err := store.Record(ctx, Record{Combo: combo(8192), VRAMMB: 6000}); err != nil {
			t.Fatal(err)
		}
	}
	input, err := store.EstimateInput(ctx, combo(8192), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Siblings) != 1 || input.Siblings[0].ContextSize != 4096 {
		t.Fatalf("siblings=%+v, want the 4096-token sample kept", input.Siblings)
	}
}

func TestEstimateInputFindsSameArchitectureOnOtherWeightsOfThisNodeOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	otherWeights := combo(8192)
	otherWeights.WeightsHash = "hash-b"
	otherNode := otherWeights
	otherNode.NodeID = "node-b"
	for _, record := range []Record{
		{Combo: otherWeights, Architecture: "llama", VRAMMB: 3300, PredictedGPUMB: 3000},
		{Combo: otherNode, Architecture: "llama", VRAMMB: 9999, PredictedGPUMB: 3000},
		{Combo: otherWeights, Architecture: "qwen3", VRAMMB: 8888, PredictedGPUMB: 3000},
	} {
		if err := store.Record(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	input, err := store.EstimateInput(ctx, combo(8192), "llama")
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Exact) != 0 || len(input.Architecture) != 1 || input.Architecture[0].VRAMMB != 3300 {
		t.Fatalf("input=%+v", input)
	}
}

func TestRecordIgnoresCombosWithoutWeightsIdentity(t *testing.T) {
	store := newTestStore(t)
	unhashed := combo(8192)
	unhashed.WeightsHash = ""
	if err := store.Record(context.Background(), Record{Combo: unhashed, VRAMMB: 1}); err != nil {
		t.Fatal(err)
	}
	input, _ := store.EstimateInput(context.Background(), combo(8192), "")
	if len(input.Exact) != 0 {
		t.Fatal("a sample without weights identity was stored")
	}
}
