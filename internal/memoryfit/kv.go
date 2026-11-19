package memoryfit

import (
	"math"
	"strings"

	"tensors-router/internal/gguf"
)

var kvBytesPerElement = map[string]float64{
	"":     2,
	"f16":  2,
	"bf16": 2,
	"f32":  4,
	"q8_0": 34.0 / 32,
	"q5_1": 24.0 / 32,
	"q5_0": 22.0 / 32,
	"q4_1": 20.0 / 32,
	"q4_0": 18.0 / 32,
}

func KVCacheMB(shape gguf.KVShape, contextSize int, typeK string, typeV string) (float64, bool) {
	if contextSize <= 0 || shape.TotalKVHeads <= 0 {
		return 0, false
	}
	bytesK, knownK := kvBytesPerElement[strings.ToLower(strings.TrimSpace(typeK))]
	bytesV, knownV := kvBytesPerElement[strings.ToLower(strings.TrimSpace(typeV))]
	if !knownK || !knownV {
		return 0, false
	}
	perToken := float64(shape.TotalKVHeads) * (float64(shape.KeyLength)*bytesK + float64(shape.ValueLength)*bytesV)
	total := perToken * float64(contextSize) / (1024 * 1024)
	if math.IsInf(total, 0) || math.IsNaN(total) {
		return 0, false
	}
	return total, true
}

func GPUFraction(gpuLayers int, blockCount int64) (float64, bool) {
	if blockCount <= 0 {
		return 0, false
	}
	switch {
	case int64(gpuLayers) > blockCount:
		return 1, true
	case gpuLayers < 0:
		return 0, false
	default:
		return float64(gpuLayers) / float64(blockCount+1), true
	}
}
