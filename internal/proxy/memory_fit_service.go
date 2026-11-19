package proxy

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/memoryfit"
)

const unknownGPULayers = -1

var koboldQuantKVCacheTypes = map[string]string{"0": "f16", "1": "q8_0", "2": "q4_0"}

func (service *Service) memoryFitTargets() []memoryFitTarget {
	if service.catalog == nil {
		return nil
	}
	models, err := service.catalog.List()
	if err != nil {
		return nil
	}
	targets := make([]memoryFitTarget, 0, len(models))
	for _, model := range models {
		if target, ok := service.memoryFitTargetFor(model); ok {
			targets = append(targets, target)
		}
	}
	return targets
}

func (service *Service) memoryFitTarget(filename string) (memoryFitTarget, bool) {
	if service.catalog == nil {
		return memoryFitTarget{}, false
	}
	models, err := service.catalog.List()
	if err != nil {
		return memoryFitTarget{}, false
	}
	for _, model := range models {
		if model.Filename == filename {
			return service.memoryFitTargetFor(model)
		}
	}
	return memoryFitTarget{}, false
}

func (service *Service) memoryFitTargetFor(model catalog.Model) (memoryFitTarget, bool) {
	if service.configDir == "" || model.Filename == "" || model.Filename != filepath.Base(model.Filename) {
		return memoryFitTarget{}, false
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, model.Filename))
	if err != nil {
		return memoryFitTarget{}, false
	}
	mode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return memoryFitTarget{}, false
	}
	gpuLayers := unknownGPULayers
	if _, configured := model.Options["gpulayers"]; configured {
		gpuLayers = int(metadata.GPULayers)
	}
	cacheTypeK, cacheTypeV := kvCacheTypes(metadata)
	modelPath := metadata.TextModelPath()
	return memoryFitTarget{
		modelID:   model.ID,
		filename:  model.Filename,
		modelPath: modelPath,
		ramOnly:   metadata.UseCPU || gpuLayers == 0,
		combo: memoryfit.Combo{
			NodeID:         service.nodeID,
			WeightsHash:    weightsIdentity(modelPath, model.ModelHash),
			BackendMode:    mode,
			GPULayers:      gpuLayers,
			KVTypeK:        cacheTypeK,
			KVTypeV:        cacheTypeV,
			FlashAttention: metadata.FlashAttention != nil && *metadata.FlashAttention,
			TensorSplit:    metadata.TensorSplitValue(),
			ContextSize:    metadata.ContextSize,
		},
	}, true
}

func weightsIdentity(modelPath string, catalogHash string) string {
	if modelPath == "" {
		return catalogHash
	}
	absolute, err := filepath.Abs(modelPath)
	if err != nil {
		return ""
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(absolute)) + "|" + strconv.FormatInt(info.Size(), 10)
}

func kvCacheTypes(metadata catalog.RuntimeConfig) (string, string) {
	quantized := strings.ToLower(strings.TrimSpace(metadata.QuantKV))
	if mapped, numbered := koboldQuantKVCacheTypes[quantized]; numbered {
		quantized = mapped
	}
	return firstCacheType(metadata.CacheTypeK, quantized), firstCacheType(metadata.CacheTypeV, quantized)
}

func firstCacheType(values ...string) string {
	for _, value := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (service *Service) memoryFreedBy(target memoryFitTarget) (int64, int64) {
	runtimes := service.runtimesReplacedBy(target)
	var vramMB, ramMB int64
	for _, runtime := range uniqueRuntimeList(runtimes) {
		runtime.state.mu.Lock()
		if runtime.state.filename != "" {
			vramMB += runtime.state.vramLoadedMB
			ramMB += runtime.state.ramLoadedMB
		}
		runtime.state.mu.Unlock()
	}
	return vramMB, ramMB
}

func (service *Service) runtimesReplacedBy(target memoryFitTarget) []*backendRuntime {
	var runtimes []*backendRuntime
	if lane, err := service.runtimeForBackendMode(target.combo.BackendMode, readinessText); err == nil && lane != nil {
		runtimes = append(runtimes, lane)
	}
	policy, err := service.resolveUnloadPolicy(target.filename)
	if err != nil || policy.IsNone() {
		return runtimes
	}
	policyRuntimes, err := service.runtimesForUnloadPolicy(target.combo.BackendMode, policy)
	if err != nil {
		return runtimes
	}
	for _, runtime := range policyRuntimes {
		if !service.separatePoolOwns(runtime) {
			runtimes = append(runtimes, runtime)
		}
	}
	return runtimes
}

func (service *Service) memoryReadings(ctx context.Context) memoryReadings {
	var readings memoryReadings
	readings.vram, readings.vramOK = service.freshNodeMemory(ctx)
	if snapshot, ok := service.nodeMemorySnapshot(ctx); ok && snapshot.RAMKnown {
		readings.ram, readings.ramOK = snapshot.RAM, true
	}
	return readings
}
