package hardware

import (
	"context"
	"sync"
	"time"
)

type VRAMSampler struct {
	source VRAMSource
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.RWMutex
	latest VRAMInfo
	valid  bool
}

func NewVRAMSampler(source VRAMSource, interval time.Duration) *VRAMSampler {
	if interval <= 0 {
		interval = time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	sampler := &VRAMSampler{
		source: source,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	if source != nil {
		sampler.capture(ctx)
		go sampler.run(ctx, interval)
	} else {
		close(sampler.done)
	}
	return sampler
}

func (sampler *VRAMSampler) VRAM(context.Context) (VRAMInfo, bool) {
	if sampler == nil {
		return VRAMInfo{}, false
	}
	sampler.mu.RLock()
	latest := sampler.latest
	valid := sampler.valid
	sampler.mu.RUnlock()
	return latest, valid
}

func (sampler *VRAMSampler) Close(ctx context.Context) error {
	if sampler == nil {
		return nil
	}
	sampler.cancel()
	select {
	case <-sampler.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (sampler *VRAMSampler) run(ctx context.Context, interval time.Duration) {
	defer close(sampler.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sampler.capture(ctx)
		}
	}
}

func (sampler *VRAMSampler) capture(ctx context.Context) {
	info, ok := sampler.source.VRAM(ctx)
	if !ok {
		return
	}
	sampler.mu.Lock()
	sampler.latest = info
	sampler.valid = true
	sampler.mu.Unlock()
}
