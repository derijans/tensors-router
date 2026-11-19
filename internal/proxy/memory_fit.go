package proxy

import (
	"context"
	"log"
	"sync"
	"time"

	"tensors-router/internal/hardware"
	"tensors-router/internal/memoryfit"
	"tensors-router/internal/memorysamples"
)

const memoryFitRecordTimeout = 30 * time.Second

type memoryFitTarget struct {
	modelID   string
	filename  string
	combo     memoryfit.Combo
	modelPath string
	ramOnly   bool
}

type memoryFitDeps interface {
	memoryFitTargets() []memoryFitTarget
	memoryFitTarget(filename string) (memoryFitTarget, bool)
	memoryFreedBy(target memoryFitTarget) (vramMB int64, ramMB int64)
	nodeMemorySnapshot(ctx context.Context) (hardware.NodeMemorySnapshot, bool)
}

type memoryReadings struct {
	vram   hardware.MemoryReading
	vramOK bool
	ram    hardware.RAMInfo
	ramOK  bool
}

type memoryFitLoad struct {
	window          *memoryFitWindow
	filename        string
	runtimeWasEmpty bool
	before          memoryReadings
}

type memoryFit struct {
	deps       memoryFitDeps
	store      *memorysamples.Store
	thresholds memoryfit.Thresholds
	logger     *log.Logger
	windows    *memoryFitWindows
	shapes     *modelShapeCache
	recording  sync.WaitGroup
}

func newMemoryFit(deps memoryFitDeps, store *memorysamples.Store, thresholds memoryfit.Thresholds, logger *log.Logger) *memoryFit {
	return &memoryFit{deps: deps, store: store, thresholds: thresholds, logger: logger, windows: newMemoryFitWindows(), shapes: newModelShapeCache()}
}

func (fit *memoryFit) beginLoad(filename string, runtimeWasEmpty bool, before memoryReadings) *memoryFitLoad {
	return &memoryFitLoad{window: fit.windows.begin(), filename: filename, runtimeWasEmpty: runtimeWasEmpty, before: before}
}

func (fit *memoryFit) runtimeChanged() {
	fit.windows.runtimeChanged()
}

func (fit *memoryFit) finishLoad(load *memoryFitLoad, after memoryReadings, loadErr error) {
	undisturbed := fit.windows.finish(load.window)
	if !undisturbed || !load.runtimeWasEmpty || loadErr != nil || fit.store == nil {
		return
	}
	target, ok := fit.deps.memoryFitTarget(load.filename)
	if !ok || !target.combo.Learnable() {
		return
	}
	vramMB, ramMB := footprintBetween(load.before, after)
	fit.recording.Go(func() { fit.record(target, vramMB, ramMB) })
}

func footprintBetween(before memoryReadings, after memoryReadings) (int64, int64) {
	var vramMB, ramMB int64
	if before.vramOK && after.vramOK && before.vram.Kind == hardware.MemoryKindVRAM && after.vram.Kind == hardware.MemoryKindVRAM {
		vramMB = max(0, after.vram.UsedMB-before.vram.UsedMB)
	}
	if before.ramOK && after.ramOK {
		ramMB = max(0, after.ram.UsedMB-before.ram.UsedMB)
	}
	return vramMB, ramMB
}

func (fit *memoryFit) record(target memoryFitTarget, vramMB int64, ramMB int64) {
	ctx, cancel := context.WithTimeout(context.Background(), memoryFitRecordTimeout)
	defer cancel()
	shape, _ := fit.shapes.shapeOf(target.modelPath)
	predicted, _ := memoryfit.PredictedGPUMB(shape.shape, target.combo)
	err := fit.store.Record(ctx, memorysamples.Record{Combo: target.combo, Architecture: shape.architecture, VRAMMB: vramMB, RAMMB: ramMB, PredictedGPUMB: predicted})
	if err != nil {
		fit.logger.Printf("memory fit sample not stored config=%q error=%v", target.filename, err)
	}
}

func (fit *memoryFit) close() {
	fit.recording.Wait()
}
