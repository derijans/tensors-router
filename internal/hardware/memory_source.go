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

type NodeMemorySnapshot struct {
	Devices   []GPUMemoryDevice
	RAM       RAMInfo
	RAMKnown  bool
	SampledAt time.Time
}

type MemorySource interface {
	Memory(context.Context) (MemoryReading, bool)
	FreshMemory(context.Context) (MemoryReading, bool)
}

type NodeMemorySnapshotSource interface {
	NodeMemory(context.Context) (NodeMemorySnapshot, bool)
}

type CachedMemorySource struct {
	vram            VRAMSource
	ram             RAMSource
	ttl             time.Duration
	now             func() time.Time
	mu              sync.Mutex
	reading         MemoryReading
	available       bool
	snapshot        NodeMemorySnapshot
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
	source.refreshIfExpiredLocked(ctx)
	return source.reading, source.available
}

func (source *CachedMemorySource) FreshMemory(ctx context.Context) (MemoryReading, bool) {
	if source == nil {
		return MemoryReading{}, false
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	source.refreshLocked(ctx)
	return source.reading, source.available
}

func (source *CachedMemorySource) NodeMemory(ctx context.Context) (NodeMemorySnapshot, bool) {
	if source == nil {
		return NodeMemorySnapshot{}, false
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	source.refreshIfExpiredLocked(ctx)
	snapshot := source.snapshot
	snapshot.Devices = append([]GPUMemoryDevice(nil), snapshot.Devices...)
	return snapshot, len(snapshot.Devices) > 0 || snapshot.RAMKnown
}

func (source *CachedMemorySource) refreshIfExpiredLocked(ctx context.Context) {
	if source.now().Before(source.expires) {
		return
	}
	source.refreshLocked(ctx)
}

func (source *CachedMemorySource) refreshLocked(ctx context.Context) {
	now := source.now()
	source.snapshot.SampledAt = now
	source.reading, source.available = source.read(ctx, now)
	source.expires = now.Add(source.ttl)
}

func (source *CachedMemorySource) read(ctx context.Context, now time.Time) (MemoryReading, bool) {
	ram, ramKnown := source.readRAM(ctx)
	source.snapshot.RAM, source.snapshot.RAMKnown = ram, ramKnown
	if source.vram != nil && !now.Before(source.nextVRAMProbeAt) {
		if info, ok := source.readVRAM(ctx); ok && info.TotalMB > 0 {
			return MemoryReading{Kind: MemoryKindVRAM, UsedMB: info.UsedMB, TotalMB: info.TotalMB, SampledAt: now}, true
		}
		source.nextVRAMProbeAt = now.Add(missingVRAMProbeInterval)
	}
	if ramKnown {
		return MemoryReading{Kind: MemoryKindRAM, UsedMB: ram.UsedMB, TotalMB: ram.TotalMB, SampledAt: now}, true
	}
	return MemoryReading{}, false
}

func (source *CachedMemorySource) readVRAM(ctx context.Context) (VRAMInfo, bool) {
	lister, listsDevices := source.vram.(DeviceMemorySource)
	if !listsDevices {
		return source.vram.VRAM(ctx)
	}
	source.snapshot.Devices = lister.DeviceMemory(ctx)
	return AggregateVRAM(source.snapshot.Devices)
}

func (source *CachedMemorySource) readRAM(ctx context.Context) (RAMInfo, bool) {
	if source.ram == nil {
		return RAMInfo{}, false
	}
	info, ok := source.ram.RAM(ctx)
	return info, ok && info.TotalMB > 0
}
