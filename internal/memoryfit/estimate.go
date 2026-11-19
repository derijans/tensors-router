package memoryfit

import (
	"math"

	"tensors-router/internal/gguf"
)

type Tier string

const (
	TierUnknown       Tier = "unknown"
	TierMeasured      Tier = "measured"
	TierContextScaled Tier = "context_scaled"
	TierArchitecture  Tier = "architecture"
)

const (
	contextScaledMargin = 0.08
	architectureMargin  = 0.15
)

type Band struct {
	LowMB  float64 `json:"low_mb"`
	HighMB float64 `json:"high_mb"`
}

type Sample struct {
	ContextSize    int
	VRAMMB         int64
	RAMMB          int64
	PredictedGPUMB float64
}

type ModelShape struct {
	KV         gguf.KVShape
	KVKnown    bool
	BlockCount int64
	FileMB     float64
}

type Estimate struct {
	Tier Tier `json:"tier"`
	VRAM Band `json:"vram"`
	RAM  Band `json:"ram"`
}

type EstimateInput struct {
	Target       Combo
	Shape        ModelShape
	Exact        []Sample
	Siblings     []Sample
	Architecture []Sample
}

func EstimateNeed(input EstimateInput) Estimate {
	if len(input.Exact) > 0 {
		return Estimate{Tier: TierMeasured, VRAM: bandOf(input.Exact, vramOf), RAM: bandOf(input.Exact, ramOf)}
	}
	if estimate, ok := contextScaledEstimate(input); ok {
		return estimate
	}
	if estimate, ok := architectureEstimate(input); ok {
		return estimate
	}
	return Estimate{Tier: TierUnknown}
}

func PredictedGPUMB(shape ModelShape, combo Combo) (float64, bool) {
	gpuFraction, fractionKnown := GPUFraction(combo.GPULayers, shape.BlockCount)
	kvMB, kvKnown := KVCacheMB(shape.KV, combo.ContextSize, combo.KVTypeK, combo.KVTypeV)
	if !fractionKnown || !kvKnown || !shape.KVKnown || shape.FileMB <= 0 {
		return 0, false
	}
	return (shape.FileMB + kvMB) * gpuFraction, true
}

func contextScaledEstimate(input EstimateInput) (Estimate, bool) {
	if len(input.Siblings) == 0 || input.Target.ContextSize <= 0 || !input.Shape.KVKnown {
		return Estimate{}, false
	}
	gpuFraction, fractionKnown := GPUFraction(input.Target.GPULayers, input.Shape.BlockCount)
	if !fractionKnown {
		return Estimate{}, false
	}
	if fitted, ok := fittedAcrossContexts(input.Siblings, input.Target.ContextSize, vramOf); ok {
		ram, _ := fittedAcrossContexts(input.Siblings, input.Target.ContextSize, ramOf)
		return Estimate{Tier: TierContextScaled, VRAM: fitted, RAM: ram}, true
	}
	nearest := nearestContext(input.Siblings, input.Target.ContextSize)
	targetKV, targetKnown := KVCacheMB(input.Shape.KV, input.Target.ContextSize, input.Target.KVTypeK, input.Target.KVTypeV)
	measuredKV, measuredKnown := KVCacheMB(input.Shape.KV, nearest, input.Target.KVTypeK, input.Target.KVTypeV)
	if !targetKnown || !measuredKnown {
		return Estimate{}, false
	}
	low, high := kvDeltaRange(targetKV-measuredKV, input.Shape.KV.Sliding)
	atContext := samplesAt(input.Siblings, nearest)
	vram := bandOf(atContext, vramOf)
	ram := bandOf(atContext, ramOf)
	return Estimate{
		Tier: TierContextScaled,
		VRAM: widened(Band{LowMB: vram.LowMB + low*gpuFraction, HighMB: vram.HighMB + high*gpuFraction}, contextScaledMargin),
		RAM:  widened(Band{LowMB: ram.LowMB + low*(1-gpuFraction), HighMB: ram.HighMB + high*(1-gpuFraction)}, contextScaledMargin),
	}, true
}

func kvDeltaRange(delta float64, sliding bool) (float64, float64) {
	if !sliding {
		return delta, delta
	}
	if delta >= 0 {
		return 0, delta
	}
	return delta, 0
}

func architectureEstimate(input EstimateInput) (Estimate, bool) {
	predicted, ok := PredictedGPUMB(input.Shape, input.Target)
	if !ok || predicted <= 0 {
		return Estimate{}, false
	}
	lowRatio, highRatio := math.Inf(1), math.Inf(-1)
	for _, sample := range input.Architecture {
		if sample.PredictedGPUMB <= 0 || sample.VRAMMB <= 0 {
			continue
		}
		ratio := float64(sample.VRAMMB) / sample.PredictedGPUMB
		lowRatio, highRatio = math.Min(lowRatio, ratio), math.Max(highRatio, ratio)
	}
	if math.IsInf(lowRatio, 1) {
		return Estimate{}, false
	}
	gpuFraction, _ := GPUFraction(input.Target.GPULayers, input.Shape.BlockCount)
	cpuShareMB := predicted / gpuFraction * (1 - gpuFraction)
	return Estimate{
		Tier: TierArchitecture,
		VRAM: widened(Band{LowMB: predicted * lowRatio, HighMB: predicted * highRatio}, architectureMargin),
		RAM:  Band{LowMB: 0, HighMB: cpuShareMB * (1 + architectureMargin)},
	}, true
}

func widened(band Band, margin float64) Band {
	return Band{LowMB: math.Max(0, band.LowMB*(1-margin)), HighMB: math.Max(0, band.HighMB*(1+margin))}
}
