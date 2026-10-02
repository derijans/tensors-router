package hardware

import (
	"context"
	"sync"
	"time"
)

const (
	MemoryKindVRAM = "vram"
	MemoryKindRAM  = "ram"
)

const (
	nodeMemoryReadingTTL     = 3 * time.Second
	missingVRAMProbeInterval = time.Minute
)

type MemoryReading struct {
	Kind      string
	UsedMB    int64
	TotalMB   int64
	SampledAt time.Time
}

type MemorySource interface {
	Memory(context.Context) (MemoryReading, bool)
	FreshMemory(context.Context) (MemoryReading, bool)
}

type CachedMemorySource struct {
	vram            VRAMSource
	ram             RAMSource
	ttl             time.Duration
	now             func() time.Time
	mu              sync.Mutex
	reading         MemoryReading
	available       bool
	expires         time.Time
	nextVRAMProbeAt time.Time
}

func NewNodeMemorySource() *CachedMemorySource {
	return NewCachedMemorySource(NewVRAMReader(), SystemRAMReader{}, nodeMemoryReadingTTL)
}

func NewCachedMemorySource(vram VRAMSource, ram RAMSource, ttl time.Duration) *CachedMemorySource {
	return &CachedMemorySource{vram: vram, ram: ram, ttl: ttl, now: time.Now}
}

func (source *CachedMemorySource) Memory(ctx context.Context) (MemoryReading, bool) {
	if source == nil {
		return MemoryReading{}, false
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.now().Before(source.expires) {
		return source.reading, source.available
	}
	return source.refreshLocked(ctx)
}

func (source *CachedMemorySource) FreshMemory(ctx context.Context) (MemoryReading, bool) {
	if source == nil {
		return MemoryReading{}, false
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.refreshLocked(ctx)
}

func (source *CachedMemorySource) refreshLocked(ctx context.Context) (MemoryReading, bool) {
	now := source.now()
	source.reading, source.available = source.read(ctx, now)
	source.expires = now.Add(source.ttl)
	return source.reading, source.available
}

func (source *CachedMemorySource) read(ctx context.Context, now time.Time) (MemoryReading, bool) {
	if source.vram != nil && !now.Before(source.nextVRAMProbeAt) {
		if info, ok := source.vram.VRAM(ctx); ok && info.TotalMB > 0 {
			return MemoryReading{Kind: MemoryKindVRAM, UsedMB: info.UsedMB, TotalMB: info.TotalMB, SampledAt: now}, true
		}
		source.nextVRAMProbeAt = now.Add(missingVRAMProbeInterval)
	}
	if source.ram != nil {
		if info, ok := source.ram.RAM(ctx); ok && info.TotalMB > 0 {
			return MemoryReading{Kind: MemoryKindRAM, UsedMB: info.UsedMB, TotalMB: info.TotalMB, SampledAt: now}, true
		}
	}
	return MemoryReading{}, false
}
