package proxy

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/hardware"
)

type deviceListingMemorySource struct {
	snapshot hardware.NodeMemorySnapshot
}

func (source deviceListingMemorySource) Memory(context.Context) (hardware.MemoryReading, bool) {
	info, ok := hardware.AggregateVRAM(source.snapshot.Devices)
	return hardware.MemoryReading{Kind: hardware.MemoryKindVRAM, UsedMB: info.UsedMB, TotalMB: info.TotalMB, SampledAt: source.snapshot.SampledAt}, ok
}

func (source deviceListingMemorySource) FreshMemory(ctx context.Context) (hardware.MemoryReading, bool) {
	return source.Memory(ctx)
}

func (source deviceListingMemorySource) NodeMemory(context.Context) (hardware.NodeMemorySnapshot, bool) {
	return source.snapshot, true
}

func twoVendorMemorySource() deviceListingMemorySource {
	return deviceListingMemorySource{snapshot: hardware.NodeMemorySnapshot{
		Devices: []hardware.GPUMemoryDevice{
			{Vendor: hardware.GPUVendorNVIDIA, Name: "RTX 3090", Source: "nvidia-smi", DedicatedTotalMB: 24576, DedicatedUsedMB: 4096, UsageKnown: true},
			{Vendor: hardware.GPUVendorIntel, Name: "Arc A770", Source: "dxgi", DedicatedTotalMB: 16288},
		},
		RAM:       hardware.RAMInfo{UsedMB: 12000, TotalMB: 65536},
		RAMKnown:  true,
		SampledAt: time.Unix(1700000000, 0),
	}}
}

func TestNodeStateListsEveryGPUAndSystemRAM(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	service.nodeMemory = twoVendorMemorySource()

	memory := service.localNodeState(context.Background()).Memory
	if memory == nil || memory.Kind != hardware.MemoryKindVRAM || memory.TotalMB != 24576 {
		t.Fatalf("aggregate memory=%+v, want only the device with known usage", memory)
	}
	if len(memory.Devices) != 2 || memory.Devices[1].Vendor != hardware.GPUVendorIntel || memory.Devices[1].UsageKnown {
		t.Fatalf("devices=%+v", memory.Devices)
	}
	if memory.SystemRAM == nil || memory.SystemRAM.TotalMB != 65536 {
		t.Fatalf("system RAM=%+v", memory.SystemRAM)
	}
}

func TestMetricsExposePerDeviceMemoryAndSkipUnknownUsage(t *testing.T) {
	service, _ := newMetricsTestService(t, true, map[string]string{"model-a": `{}`})
	service.nodeMemory = twoVendorMemorySource()

	_, body := scrape(t, service)
	for _, want := range []string{
		`tensors_router_gpu_memory_dedicated_used_bytes{device="gpu0",vendor="nvidia",name="RTX 3090"} 4294967296`,
		`tensors_router_gpu_memory_dedicated_total_bytes{device="gpu1",vendor="intel",name="Arc A770"} 17079205888`,
		`tensors_router_system_memory_total_bytes 68719476736`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, `dedicated_used_bytes{device="gpu1"`) {
		t.Fatalf("a device with unknown usage reported a used value:\n%s", body)
	}
}
