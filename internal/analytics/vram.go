package analytics

import (
	"context"
	"sync"
	"time"

	"tensors-router/internal/hardware"
)

type EventFinalizer = func(*Event)

type VRAMLoadMeasurement struct {
	startedAt  time.Time
	finishedAt time.Time
	before     hardware.VRAMInfo
	after      hardware.VRAMInfo
	hasBefore  bool
	hasAfter   bool
}

func StartVRAMLoad(ctx context.Context, source hardware.VRAMSource) *VRAMLoadMeasurement {
	measurement := &VRAMLoadMeasurement{startedAt: time.Now()}
	if source != nil {
		measurement.before, measurement.hasBefore = source.VRAM(ctx)
	}
	return measurement
}

func (measurement *VRAMLoadMeasurement) Finish(ctx context.Context, source hardware.VRAMSource) {
	measurement.finishedAt = time.Now()
	if source != nil {
		measurement.after, measurement.hasAfter = source.VRAM(ctx)
	}
}

func (measurement *VRAMLoadMeasurement) Baseline() (hardware.VRAMInfo, bool) {
	return measurement.before, measurement.hasBefore
}

func (measurement *VRAMLoadMeasurement) ApplyTo(event *Event) {
	if measurement.finishedAt.IsZero() {
		measurement.finishedAt = time.Now()
	}
	event.StartedAt = measurement.startedAt
	event.FinishedAt = measurement.finishedAt
	event.DurationMS = measurement.finishedAt.Sub(measurement.startedAt).Milliseconds()
	if measurement.hasBefore {
		event.LoadVRAMBefore = measurement.before.UsedMB
		event.VRAMTotal = measurement.before.TotalMB
	}
	if measurement.hasAfter {
		event.LoadVRAMAfter = measurement.after.UsedMB
		event.VRAMTotal = measurement.after.TotalMB
		event.VRAMPeakPercent = measurement.after.UsedPercent
	}
}

type VRAMWorkSampler struct {
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

func StartVRAMWorkSampler(source hardware.VRAMSource, interval time.Duration, baselineMB int64, hasBaseline bool) *VRAMWorkSampler {
	sampler := &VRAMWorkSampler{
		source:      source,
		interval:    interval,
		baselineMB:  baselineMB,
		hasBaseline: hasBaseline,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	sampler.recordSample()
	go sampler.run()
	return sampler
}

func (sampler *VRAMWorkSampler) run() {
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

func (sampler *VRAMWorkSampler) Finish(event *Event) {
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

func (sampler *VRAMWorkSampler) recordSample() {
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
