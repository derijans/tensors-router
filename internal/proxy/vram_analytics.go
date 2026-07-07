package proxy

import (
	"context"
	"sync"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/hardware"
)

type analyticsEventFinalizer = func(*routeranalytics.Event)

type vramLoadMeasurement struct {
	startedAt  time.Time
	finishedAt time.Time
	before     hardware.VRAMInfo
	after      hardware.VRAMInfo
	hasBefore  bool
	hasAfter   bool
}

type vramWorkSampler struct {
	source      hardware.VRAMSource
	interval    time.Duration
	baselineMB  int64
	hasBaseline bool
	stop        chan struct{}
	done        chan struct{}
	once        sync.Once
	mu          sync.Mutex
	start       hardware.VRAMInfo
	max         hardware.VRAMInfo
	end         hardware.VRAMInfo
	hasStart    bool
	hasMax      bool
	hasEnd      bool
}

func (service *Service) vramAnalyticsActive() bool {
	return service.analyticsStore != nil && service.vramAnalyticsEnabled && service.vramSource != nil
}

func (service *Service) beginVRAMLoad(ctx context.Context) *vramLoadMeasurement {
	if !service.vramAnalyticsActive() {
		return nil
	}
	measurement := &vramLoadMeasurement{startedAt: time.Now()}
	measurement.before, measurement.hasBefore = service.sampleVRAM(ctx)
	return measurement
}

func (service *Service) finishVRAMLoad(ctx context.Context, measurement *vramLoadMeasurement) {
	if measurement == nil {
		return
	}
	measurement.finishedAt = time.Now()
	measurement.after, measurement.hasAfter = service.sampleVRAM(ctx)
}

func (service *Service) recordVRAMLoad(modelID string, configFilename string, readiness backendReadiness, backendMode string, measurement *vramLoadMeasurement) {
	if measurement == nil || service.analyticsStore == nil {
		return
	}
	if measurement.finishedAt.IsZero() {
		measurement.finishedAt = time.Now()
	}
	event := routeranalytics.Event{
		NodeID:         service.nodeID,
		ModelID:        modelID,
		Section:        readinessAnalyticsSection(readiness),
		BackendMode:    backendMode,
		EventType:      routeranalytics.EventTypeModelLoad,
		Route:          "model_load",
		ConfigFilename: configFilename,
		StatusCode:     200,
		Success:        true,
		StartedAt:      measurement.startedAt,
		FinishedAt:     measurement.finishedAt,
		DurationMS:     measurement.finishedAt.Sub(measurement.startedAt).Milliseconds(),
	}
	if measurement.hasBefore {
		event.LoadVRAMBefore = measurement.before.UsedMB
		event.VRAMTotal = measurement.before.TotalMB
	}
	if measurement.hasAfter {
		event.LoadVRAMAfter = measurement.after.UsedMB
		event.VRAMTotal = measurement.after.TotalMB
		event.VRAMPeakPercent = measurement.after.UsedPercent
	}
	service.analyticsStore.Record(event)
}

func applyVRAMLoadStateLocked(state *activeConfigState, measurement *vramLoadMeasurement) {
	state.vramBaselineValid = false
	state.vramBaselineMB = 0
	state.vramTotalMB = 0
	if measurement == nil || !measurement.hasBefore {
		return
	}
	state.vramBaselineValid = true
	state.vramBaselineMB = measurement.before.UsedMB
	state.vramTotalMB = measurement.before.TotalMB
}

func clearVRAMLoadStateLocked(state *activeConfigState) {
	state.vramBaselineValid = false
	state.vramBaselineMB = 0
	state.vramTotalMB = 0
}

func (service *Service) beginVRAMWork(runtime *backendRuntime) analyticsEventFinalizer {
	if !service.vramAnalyticsActive() || runtime == nil {
		return nil
	}
	baselineMB, hasBaseline := runtimeVRAMBaseline(runtime)
	sampler := &vramWorkSampler{
		source:      service.vramSource,
		interval:    service.vramSampleInterval,
		baselineMB:  baselineMB,
		hasBaseline: hasBaseline,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	sampler.recordSample()
	go sampler.run()
	return sampler.finish
}

func runtimeVRAMBaseline(runtime *backendRuntime) (int64, bool) {
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	return runtime.state.vramBaselineMB, runtime.state.vramBaselineValid
}

func (sampler *vramWorkSampler) run() {
	ticker := time.NewTicker(sampler.interval)
	defer ticker.Stop()
	defer close(sampler.done)
	for {
		select {
		case <-ticker.C:
			sampler.recordSample()
		case <-sampler.stop:
			return
		}
	}
}

func (sampler *vramWorkSampler) finish(event *routeranalytics.Event) {
	if sampler == nil || event == nil {
		return
	}
	sampler.once.Do(func() {
		close(sampler.stop)
		<-sampler.done
		sampler.recordSample()
		sampler.mu.Lock()
		defer sampler.mu.Unlock()
		if sampler.hasStart {
			event.WorkVRAMStart = sampler.start.UsedMB
		}
		if sampler.hasMax {
			event.WorkVRAMMax = sampler.max.UsedMB
			event.VRAMPeakPercent = sampler.max.UsedPercent
			event.VRAMTotal = sampler.max.TotalMB
			if sampler.hasBaseline && sampler.max.UsedMB > sampler.baselineMB {
				event.ModelVRAM = sampler.max.UsedMB - sampler.baselineMB
			}
		}
		if sampler.hasEnd {
			event.WorkVRAMEnd = sampler.end.UsedMB
			if event.VRAMTotal == 0 {
				event.VRAMTotal = sampler.end.TotalMB
			}
		}
	})
}

func (sampler *vramWorkSampler) recordSample() {
	info, ok := sampler.source.VRAM(context.Background())
	if !ok {
		return
	}
	sampler.mu.Lock()
	defer sampler.mu.Unlock()
	if !sampler.hasStart {
		sampler.start = info
		sampler.hasStart = true
	}
	if !sampler.hasMax || info.UsedMB > sampler.max.UsedMB {
		sampler.max = info
		sampler.hasMax = true
	}
	sampler.end = info
	sampler.hasEnd = true
}

func (service *Service) sampleVRAM(ctx context.Context) (hardware.VRAMInfo, bool) {
	if service.vramSource == nil {
		return hardware.VRAMInfo{}, false
	}
	return service.vramSource.VRAM(ctx)
}

func readinessAnalyticsSection(readiness backendReadiness) string {
	if readiness == readinessImage {
		return routeranalytics.SectionImage
	}
	return routeranalytics.SectionLLM
}
