package proxy

import (
	"context"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/hardware"
)

type modelLoadMeasurement struct {
	analytics      *routeranalytics.VRAMLoadMeasurement
	memoryBefore   hardware.MemoryReading
	memoryAfter    hardware.MemoryReading
	memoryBeforeOK bool
	memoryAfterOK  bool
}

func (service *Service) beginModelLoad(ctx context.Context) *modelLoadMeasurement {
	measurement := &modelLoadMeasurement{analytics: service.analytics.beginLoad(ctx)}
	measurement.memoryBefore, measurement.memoryBeforeOK = service.freshNodeMemory(ctx)
	return measurement
}

func (service *Service) finishModelLoad(ctx context.Context, measurement *modelLoadMeasurement) {
	service.analytics.finishLoad(ctx, measurement.analytics)
	measurement.memoryAfter, measurement.memoryAfterOK = service.freshNodeMemory(ctx)
}

func (service *Service) freshNodeMemory(ctx context.Context) (hardware.MemoryReading, bool) {
	if service.nodeMemory == nil {
		return hardware.MemoryReading{}, false
	}
	return service.nodeMemory.FreshMemory(ctx)
}

func (measurement *modelLoadMeasurement) memoryFootprintMB() int64 {
	if !measurement.memoryBeforeOK || !measurement.memoryAfterOK || measurement.memoryBefore.Kind != measurement.memoryAfter.Kind {
		return 0
	}
	return max(0, measurement.memoryAfter.UsedMB-measurement.memoryBefore.UsedMB)
}

func applyLoadMeasurementLocked(state *activeConfigState, measurement *modelLoadMeasurement) {
	clearLoadMeasurementLocked(state)
	state.memoryLoadedMB = measurement.memoryFootprintMB()
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
}
