package proxy

import (
	"context"
	"math"

	"tensors-router/internal/hardware"
	"tensors-router/internal/memoryfit"
	"tensors-router/internal/siteapi"
)

const (
	memoryFitNoStore  = "memory samples are stored only when the router store is open"
	memoryFitNoMemory = "this node reports no memory readings"
)

type nodeCapacity struct {
	gpuFreeMB float64
	ramFreeMB float64
	hasGPU    bool
	multiGPU  bool
	known     bool
}

func capacityOf(snapshot hardware.NodeMemorySnapshot, known bool) nodeCapacity {
	capacity := nodeCapacity{known: known}
	gpus := 0
	for _, device := range snapshot.Devices {
		if !device.UsageKnown || device.Unified || device.DedicatedTotalMB <= 0 {
			continue
		}
		gpus++
		capacity.gpuFreeMB += float64(max(0, device.DedicatedTotalMB-device.DedicatedUsedMB))
	}
	capacity.hasGPU, capacity.multiGPU = gpus > 0, gpus > 1
	if snapshot.RAMKnown {
		capacity.ramFreeMB = float64(max(0, snapshot.RAM.TotalMB-snapshot.RAM.UsedMB))
	}
	return capacity
}

func (fit *memoryFit) nodeVerdicts(ctx context.Context, nodeID string) siteapi.NodeMemoryFit {
	snapshot, known := fit.deps.nodeMemorySnapshot(ctx)
	capacity := capacityOf(snapshot, known)
	targets := fit.deps.memoryFitTargets()
	result := siteapi.NodeMemoryFit{NodeID: nodeID, Configs: make([]siteapi.ConfigMemoryFit, 0, len(targets))}
	for _, target := range targets {
		result.Configs = append(result.Configs, fit.verdictFor(ctx, target, capacity))
	}
	return result
}

func (fit *memoryFit) verdictFor(ctx context.Context, target memoryFitTarget, capacity nodeCapacity) siteapi.ConfigMemoryFit {
	view := siteapi.ConfigMemoryFit{ModelID: target.modelID, ConfigName: target.filename, Level: string(memoryfit.LevelUnknown), Tier: string(memoryfit.TierUnknown)}
	switch {
	case fit.store == nil:
		view.Unavailable = memoryFitNoStore
		return view
	case !capacity.known:
		view.Unavailable = memoryFitNoMemory
		return view
	}
	shape, _ := fit.shapes.shapeOf(target.modelPath)
	input, err := fit.store.EstimateInput(ctx, target.combo, shape.architecture)
	if err != nil {
		view.Unavailable = err.Error()
		return view
	}
	input.Shape = shape.shape
	estimate := memoryfit.EstimateNeed(input)
	freedVRAM, freedRAM := fit.deps.memoryFreedBy(target)
	limits := memoryfit.Capacity{
		GPUFreeMB: capacity.gpuFreeMB + float64(freedVRAM),
		RAMFreeMB: capacity.ramFreeMB + float64(freedRAM),
		RAMOnly:   target.ramOnly || !capacity.hasGPU,
	}
	verdict := memoryfit.Classify(estimate, limits, fit.thresholds)
	view.Level, view.Tier = string(verdict.Level), string(estimate.Tier)
	view.VRAM, view.RAM = bandView(estimate.VRAM), bandView(estimate.RAM)
	view.GPUFreeMB, view.RAMFreeMB, view.RAMOnly = limits.GPUFreeMB, limits.RAMFreeMB, limits.RAMOnly
	view.SpillMB, view.MultiGPU = verdict.SpillMB, capacity.multiGPU
	return view
}

func bandView(band memoryfit.Band) siteapi.MemoryFitBand {
	return siteapi.MemoryFitBand{LowMB: math.Round(band.LowMB), HighMB: math.Round(band.HighMB)}
}
