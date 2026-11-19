package hardware

import (
	"context"
	"testing"
	"time"
)

type listingVRAMSource struct {
	devices []GPUMemoryDevice
	calls   int
}

func (source *listingVRAMSource) VRAM(ctx context.Context) (VRAMInfo, bool) {
	return AggregateVRAM(source.DeviceMemory(ctx))
}

func (source *listingVRAMSource) DeviceMemory(context.Context) []GPUMemoryDevice {
	source.calls++
	return source.devices
}

func TestNodeMemorySnapshotListsDevicesAndRAMFromOneProbe(t *testing.T) {
	clock := time.Unix(1000, 0)
	vram := &listingVRAMSource{devices: []GPUMemoryDevice{
		{Vendor: GPUVendorNVIDIA, Name: "RTX", DedicatedTotalMB: 8192, DedicatedUsedMB: 1024, UsageKnown: true},
		{Vendor: GPUVendorAMD, Name: "Radeon", DedicatedTotalMB: 16384, DedicatedUsedMB: 2048, UsageKnown: true},
	}}
	source := memorySourceAt(vram, fixedRAMSource{info: RAMInfo{UsedMB: 8000, TotalMB: 65536}, ok: true}, &clock)

	reading, ok := source.Memory(context.Background())
	snapshot, snapshotOK := source.NodeMemory(context.Background())
	if !ok || reading.Kind != MemoryKindVRAM || reading.TotalMB != 24576 || reading.UsedMB != 3072 {
		t.Fatalf("aggregate reading=%+v", reading)
	}
	if !snapshotOK || len(snapshot.Devices) != 2 || !snapshot.RAMKnown || snapshot.RAM.TotalMB != 65536 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if vram.calls != 1 {
		t.Fatalf("devices probed %d times for one cached reading", vram.calls)
	}
}

func TestNodeMemorySnapshotIsACopy(t *testing.T) {
	clock := time.Unix(1000, 0)
	vram := &listingVRAMSource{devices: []GPUMemoryDevice{{Name: "RTX", DedicatedTotalMB: 8192, UsageKnown: true}}}
	source := memorySourceAt(vram, fixedRAMSource{}, &clock)

	first, _ := source.NodeMemory(context.Background())
	first.Devices[0].Name = "changed"
	second, _ := source.NodeMemory(context.Background())
	if second.Devices[0].Name != "RTX" {
		t.Fatal("a caller changed the cached device list")
	}
}
