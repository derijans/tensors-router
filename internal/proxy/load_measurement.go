package proxy

import (
	"context"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/hardware"
)

type modelLoadMeasurement struct {
	started   time.Time
	analytics *routeranalytics.VRAMLoadMeasurement
	before    memoryReadings
	after     memoryReadings
	fit       *memoryFitLoad
}

func (service *Service) beginModelLoad(ctx context.Context, runtime *backendRuntime, configFilename string) *modelLoadMeasurement {
	measurement := &modelLoadMeasurement{analytics: service.analytics.beginLoad(ctx)}
	measurement.before = service.memoryReadings(ctx)
	measurement.fit = service.memoryFit.beginLoad(configFilename, runtimeHoldsNothing(runtime), measurement.before)
	measurement.started = time.Now()
	return measurement
}

func (service *Service) finishModelLoad(ctx context.Context, runtime *backendRuntime, measurement *modelLoadMeasurement, loadErr error) {
	service.metrics.modelLoadFinished(runtime.mode, time.Since(measurement.started), loadErr)
	service.analytics.finishLoad(ctx, measurement.analytics)
	measurement.after = service.memoryReadings(ctx)
	service.memoryFit.finishLoad(measurement.fit, measurement.after, loadErr)
}

func runtimeHoldsNothing(runtime *backendRuntime) bool {
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	return runtime.state.filename == "" && runtime.state.physicalFilename == ""
}

func (service *Service) freshNodeMemory(ctx context.Context) (hardware.MemoryReading, bool) {
	if service.nodeMemory == nil {
		return hardware.MemoryReading{}, false
	}
	return service.nodeMemory.FreshMemory(ctx)
}

func (measurement *modelLoadMeasurement) memoryFootprintMB() int64 {
	before, after := measurement.before, measurement.after
	if !before.vramOK || !after.vramOK || before.vram.Kind != after.vram.Kind {
		return 0
	}
	return max(0, after.vram.UsedMB-before.vram.UsedMB)
}

func applyLoadMeasurementLocked(state *activeConfigState, measurement *modelLoadMeasurement) {
	clearLoadMeasurementLocked(state)
	state.memoryLoadedMB = measurement.memoryFootprintMB()
	state.vramLoadedMB, state.ramLoadedMB = footprintBetween(measurement.before, measurement.after)
	if measurement.analytics == nil {
		return
	}
	baseline, measured := measurement.analytics.Baseline()
	if !measured {
		return
	}
	state.vramBaselineValid = true
	state.vramBaselineMB = baseline.UsedMB
	state.vramTotalMB = baseline.TotalMB
}

func clearLoadMeasurementLocked(state *activeConfigState) {
	state.vramBaselineValid = false
	state.vramBaselineMB = 0
	state.vramTotalMB = 0
	state.memoryLoadedMB = 0
	state.vramLoadedMB = 0
	state.ramLoadedMB = 0
}
