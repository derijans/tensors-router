package memoryfit

import (
	"math"
	"testing"

	"tensors-router/internal/gguf"
)

func llama8BShape() ModelShape {
	return ModelShape{KV: gguf.KVShape{TotalKVHeads: 32 * 8, KeyLength: 128, ValueLength: 128}, KVKnown: true, BlockCount: 32, FileMB: 4700}
}

func TestKVCacheMatchesTheKnownLlama3Size(t *testing.T) {
	kv, ok := KVCacheMB(llama8BShape().KV, 8192, "f16", "f16")
	if !ok || kv != 1024 {
		t.Fatalf("kv=%v ok=%t, want 1024 MB for Llama 3 8B at 8192 tokens in f16", kv, ok)
	}
	quantized, _ := KVCacheMB(llama8BShape().KV, 8192, "q8_0", "q8_0")
	if math.Abs(quantized-544) > 0.01 {
		t.Fatalf("q8_0 kv=%v, want 544 MB", quantized)
	}
	if _, ok := KVCacheMB(llama8BShape().KV, 8192, "q3_k", "f16"); ok {
		t.Fatal("an unknown cache type produced a size")
	}
}

func TestEstimateUsesMeasuredSamplesFirst(t *testing.T) {
	estimate := EstimateNeed(EstimateInput{
		Target: Combo{ContextSize: 8192, GPULayers: 99},
		Shape:  llama8BShape(),
		Exact:  []Sample{{ContextSize: 8192, VRAMMB: 6100, RAMMB: 300}, {ContextSize: 8192, VRAMMB: 6300, RAMMB: 250}},
	})
	if estimate.Tier != TierMeasured || estimate.VRAM != (Band{LowMB: 6100, HighMB: 6300}) || estimate.RAM != (Band{LowMB: 250, HighMB: 300}) {
		t.Fatalf("estimate=%+v", estimate)
	}
}

func TestEstimateScalesOneMeasuredContextByTheKVDelta(t *testing.T) {
	estimate := EstimateNeed(EstimateInput{
		Target:   Combo{ContextSize: 16384, GPULayers: 99},
		Shape:    llama8BShape(),
		Siblings: []Sample{{ContextSize: 8192, VRAMMB: 6000}},
	})
	if estimate.Tier != TierContextScaled {
		t.Fatalf("tier=%s", estimate.Tier)
	}
	if math.Abs(estimate.VRAM.LowMB-7024*0.92) > 0.5 || math.Abs(estimate.VRAM.HighMB-7024*1.08) > 0.5 {
		t.Fatalf("vram band=%+v, want 7024 MB widened by 8%%", estimate.VRAM)
	}
}

func TestEstimateDoesNotCountSlidingWindowKVInTheLowEnd(t *testing.T) {
	shape := llama8BShape()
	shape.KV.Sliding = true
	estimate := EstimateNeed(EstimateInput{
		Target:   Combo{ContextSize: 131072, GPULayers: 99},
		Shape:    shape,
		Siblings: []Sample{{ContextSize: 8192, VRAMMB: 6000}},
	})
	if estimate.VRAM.LowMB > 6000 {
		t.Fatalf("low end=%v grew with a KV size that sliding-window layers may never allocate", estimate.VRAM.LowMB)
	}
}

func TestEstimateFitsALineAcrossMeasuredContexts(t *testing.T) {
	estimate := EstimateNeed(EstimateInput{
		Target: Combo{ContextSize: 32768, GPULayers: 99},
		Shape:  llama8BShape(),
		Siblings: []Sample{
			{ContextSize: 4096, VRAMMB: 5200},
			{ContextSize: 8192, VRAMMB: 5700},
			{ContextSize: 16384, VRAMMB: 6700},
		},
	})
	predicted := 4700.0 + 0.1220703125*32768
	if estimate.Tier != TierContextScaled || estimate.VRAM.LowMB > predicted || estimate.VRAM.HighMB < predicted {
		t.Fatalf("band=%+v should contain the extrapolated %v", estimate.VRAM, predicted)
	}
}

func TestEstimateLearnsFromTheSameArchitectureWithWiderBands(t *testing.T) {
	target := Combo{ContextSize: 8192, GPULayers: 99}
	predicted, _ := PredictedGPUMB(llama8BShape(), target)
	estimate := EstimateNeed(EstimateInput{
		Target:       target,
		Shape:        llama8BShape(),
		Architecture: []Sample{{VRAMMB: 3300, PredictedGPUMB: 3000}, {VRAMMB: 6600, PredictedGPUMB: 6000}},
	})
	if estimate.Tier != TierArchitecture {
		t.Fatalf("tier=%s", estimate.Tier)
	}
	if math.Abs(estimate.VRAM.LowMB-predicted*1.1*0.85) > 0.5 || math.Abs(estimate.VRAM.HighMB-predicted*1.1*1.15) > 0.5 {
		t.Fatalf("band=%+v, want the 1.1 ratio widened by 15%%", estimate.VRAM)
	}
	if estimate.RAM.LowMB != 0 {
		t.Fatalf("RAM low=%v must stay 0 for an unmeasured model", estimate.RAM.LowMB)
	}
}

func TestEstimateIsUnknownWithoutAnyRelatedSample(t *testing.T) {
	if estimate := EstimateNeed(EstimateInput{Target: Combo{ContextSize: 8192, GPULayers: 99}, Shape: llama8BShape()}); estimate.Tier != TierUnknown {
		t.Fatalf("estimate=%+v", estimate)
	}
}

func TestComboKeysSeparateContextFromPlacement(t *testing.T) {
	base := Combo{NodeID: "n", WeightsHash: "h", BackendMode: "kobold", GPULayers: 99, ContextSize: 4096}
	other := base
	other.ContextSize = 8192
	if base.Key() == other.Key() || base.PlacementKey() != other.PlacementKey() {
		t.Fatal("context must change the combo key but not the placement key")
	}
	renamed := base
	renamed.WeightsHash = "H"
	if renamed.PlacementKey() == base.PlacementKey() {
		t.Fatal("weights identity is case-sensitive")
	}
}
