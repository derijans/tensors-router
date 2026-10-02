package hardware

import (
	"context"
	"testing"
	"time"
)

type scriptedVRAMSource struct {
	info  VRAMInfo
	ok    bool
	calls int
}

func (source *scriptedVRAMSource) VRAM(context.Context) (VRAMInfo, bool) {
	source.calls++
	return source.info, source.ok
}

type fixedRAMSource struct {
	info RAMInfo
	ok   bool
}

func (source fixedRAMSource) RAM(context.Context) (RAMInfo, bool) {
	return source.info, source.ok
}

func memorySourceAt(vram VRAMSource, ram RAMSource, clock *time.Time) *CachedMemorySource {
	source := NewCachedMemorySource(vram, ram, 3*time.Second)
	source.now = func() time.Time { return *clock }
	return source
}

func TestCachedMemorySourceReusesVRAMReadingUntilItExpires(t *testing.T) {
	clock := time.Unix(1000, 0)
	vram := &scriptedVRAMSource{info: VRAMInfo{UsedMB: 4000, TotalMB: 24000}, ok: true}
	source := memorySourceAt(vram, fixedRAMSource{info: RAMInfo{UsedMB: 1, TotalMB: 2}, ok: true}, &clock)

	first, ok := source.Memory(context.Background())
	clock = clock.Add(2 * time.Second)
	second, _ := source.Memory(context.Background())
	if !ok || first.Kind != MemoryKindVRAM || first.UsedMB != 4000 || first.TotalMB != 24000 || second != first || vram.calls != 1 {
		t.Fatalf("expected one cached VRAM reading, got %#v %#v after %d reads", first, second, vram.calls)
	}

	vram.info.UsedMB = 9000
	clock = clock.Add(2 * time.Second)
	refreshed, _ := source.Memory(context.Background())
	if refreshed.UsedMB != 9000 || !refreshed.SampledAt.Equal(clock) || vram.calls != 2 {
		t.Fatalf("expected refreshed reading after the TTL, got %#v after %d reads", refreshed, vram.calls)
	}

	vram.info.UsedMB = 12000
	fresh, _ := source.FreshMemory(context.Background())
	cached, _ := source.Memory(context.Background())
	if fresh.UsedMB != 12000 || cached != fresh || vram.calls != 3 {
		t.Fatalf("expected a fresh read inside the TTL to replace the cache, got %#v %#v after %d reads", fresh, cached, vram.calls)
	}
}

func TestCachedMemorySourceFallsBackToRAMAndProbesMissingVRAMRarely(t *testing.T) {
	clock := time.Unix(1000, 0)
	vram := &scriptedVRAMSource{}
	source := memorySourceAt(vram, fixedRAMSource{info: RAMInfo{UsedMB: 61000, TotalMB: 196608}, ok: true}, &clock)

	reading, ok := source.Memory(context.Background())
	if !ok || reading.Kind != MemoryKindRAM || reading.UsedMB != 61000 || reading.TotalMB != 196608 {
		t.Fatalf("expected RAM fallback, got %#v ok=%v", reading, ok)
	}
	source.FreshMemory(context.Background())
	if vram.calls != 1 {
		t.Fatalf("expected a missing VRAM tool to be probed once per interval, probed %d times", vram.calls)
	}
	clock = clock.Add(missingVRAMProbeInterval)
	source.FreshMemory(context.Background())
	if vram.calls != 2 {
		t.Fatalf("expected VRAM to be probed again after the interval, probed %d times", vram.calls)
	}
}

func TestCachedMemorySourceReportsUnavailableWhenNothingReads(t *testing.T) {
	clock := time.Unix(1000, 0)
	source := memorySourceAt(&scriptedVRAMSource{}, fixedRAMSource{}, &clock)

	if reading, ok := source.Memory(context.Background()); ok {
		t.Fatalf("expected unavailable memory, got %#v", reading)
	}
}

func TestParseMeminfoUsesAvailableMemory(t *testing.T) {
	info, ok := parseMeminfo([]byte("MemTotal:       16384000 kB\nMemFree:         1024000 kB\nMemAvailable:    4096000 kB\n"))
	if !ok || info.TotalMB != 16000 || info.UsedMB != 12000 {
		t.Fatalf("unexpected meminfo parse %#v ok=%v", info, ok)
	}
	if _, ok := parseMeminfo([]byte("MemTotal: 16384000 kB\n")); ok {
		t.Fatalf("expected meminfo without MemAvailable to be rejected")
	}
}

func TestParseVMStatCountsFreeInactiveAndSpeculativePagesAsAvailable(t *testing.T) {
	output := []byte("Mach Virtual Memory Statistics: (page size of 16384 bytes)\n" +
		"Pages free:                               65536.\n" +
		"Pages active:                            900000.\n" +
		"Pages inactive:                          131072.\n" +
		"Pages speculative:                        65536.\n" +
		"Pages wired down:                        200000.\n")
	info, ok := parseVMStat(output, 32*1024*bytesPerMB)
	if !ok || info.TotalMB != 32*1024 || info.UsedMB != 28*1024 {
		t.Fatalf("unexpected vm_stat parse %#v ok=%v", info, ok)
	}
	if _, ok := parseVMStat([]byte("Pages free: 1.\n"), 1024); ok {
		t.Fatalf("expected vm_stat without page size to be rejected")
	}
}
