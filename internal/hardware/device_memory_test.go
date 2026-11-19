package hardware

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"
)

type fixedProbe struct {
	found []GPUMemoryDevice
	err   error
}

func (probe fixedProbe) devices(context.Context) ([]GPUMemoryDevice, error) {
	return probe.found, probe.err
}

func TestParseNvidiaDevicesKeepsGoodRowsAndDropsMalformedOnes(t *testing.T) {
	devices := parseNvidiaDevices([]byte("0, 00000000:01:00.0, RTX 4090, 1200, 24564\r\nbroken row\n1, 00000000:02:00.0, RTX 3060, 0, 12288\n2, 00000000:03:00.0, Bad, x, 8192\n"))
	if len(devices) != 2 {
		t.Fatalf("devices=%+v, want the two well-formed rows", devices)
	}
	if devices[0].Name != "RTX 4090" || devices[0].PCIBusID != "00000000:01:00.0" || devices[0].DedicatedUsedMB != 1200 || !devices[0].UsageKnown {
		t.Fatalf("first device=%+v", devices[0])
	}
}

func TestParseROCmDevicesReportsEachCard(t *testing.T) {
	devices := parseROCmDevices([]byte("device,VRAM Total Memory (B),VRAM Total Used Memory (B)\ncard0,8589934592,1073741824\ncard1,17179869184,2147483648\n"))
	if len(devices) != 2 || devices[1].Name != "card1" || devices[1].DedicatedTotalMB != 16384 || devices[1].DedicatedUsedMB != 2048 {
		t.Fatalf("devices=%+v", devices)
	}
}

func amdgpuTestSysfs() fstest.MapFS {
	card := func(vendor string, total string, used string, gtt string, slot string) map[string]*fstest.MapFile {
		return map[string]*fstest.MapFile{
			"vendor":              {Data: []byte(vendor + "\n")},
			"mem_info_vram_total": {Data: []byte(total + "\n")},
			"mem_info_vram_used":  {Data: []byte(used + "\n")},
			"mem_info_gtt_used":   {Data: []byte(gtt + "\n")},
			"uevent":              {Data: []byte("DRIVER=amdgpu\nPCI_SLOT_NAME=" + slot + "\n")},
		}
	}
	files := fstest.MapFS{"class/drm/card0-DP-1/status": {Data: []byte("connected\n")}}
	for name, content := range map[string]map[string]*fstest.MapFile{
		"card0": card("0x1002", "25753026560", "1073741824", "52428800", "0000:03:00.0"),
		"card1": card("0x8086", "0", "0", "0", "0000:00:02.0"),
		"card2": card("0x1002", "536870912", "268435456", "4294967296", "0000:c5:00.0"),
	} {
		for file, data := range content {
			files["class/drm/"+name+"/device/"+file] = data
		}
	}
	return files
}

func TestAMDGPUSysfsReadsOnlyAMDCardsAndSkipsConnectors(t *testing.T) {
	devices, err := amdgpuSysfsProbe{sysfs: amdgpuTestSysfs()}.devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices=%+v, want the two AMD cards only", devices)
	}
	byBus := map[string]GPUMemoryDevice{}
	for _, device := range devices {
		byBus[device.PCIBusID] = device
	}
	discrete := byBus["0000:03:00.0"]
	if discrete.DedicatedTotalMB != 24560 || discrete.DedicatedUsedMB != 1024 || discrete.SharedUsedMB != 50 || !discrete.UsageKnown {
		t.Fatalf("discrete card=%+v", discrete)
	}
	if apu := byBus["0000:c5:00.0"]; apu.SharedUsedMB != 4096 {
		t.Fatalf("APU GTT usage not reported as shared: %+v", apu)
	}
}

func TestAMDGPUSysfsIsUnavailableWithoutDRM(t *testing.T) {
	if _, err := (amdgpuSysfsProbe{sysfs: fstest.MapFS{}}).devices(context.Background()); err == nil {
		t.Fatal("a missing class/drm must report the probe as unavailable")
	}
}

func TestMergeLetsTheFirstProbeOwnAVendor(t *testing.T) {
	sysfsAMD := GPUMemoryDevice{Vendor: GPUVendorAMD, PCIBusID: "0000:03:00.0", Source: amdgpuSysfsSource, DedicatedTotalMB: 24560, DedicatedUsedMB: 1024, UsageKnown: true}
	rocmAMD := GPUMemoryDevice{Vendor: GPUVendorAMD, Name: "card0", Source: rocmSMIDeviceSource, DedicatedTotalMB: 24560, DedicatedUsedMB: 1100, UsageKnown: true}
	nvidia := GPUMemoryDevice{Vendor: GPUVendorNVIDIA, PCIBusID: "00000000:01:00.0", DedicatedTotalMB: 8192, DedicatedUsedMB: 512, UsageKnown: true}

	merged := mergeProbeResults(context.Background(), []deviceProbe{
		fixedProbe{found: []GPUMemoryDevice{nvidia}},
		fixedProbe{err: errors.New("no sysfs")},
		fixedProbe{found: []GPUMemoryDevice{sysfsAMD}},
		fixedProbe{found: []GPUMemoryDevice{rocmAMD, nvidia}},
	})
	if len(merged) != 2 {
		t.Fatalf("merged=%+v, want one NVIDIA and one AMD device", merged)
	}
	info, ok := AggregateVRAM(merged)
	if !ok || info.TotalMB != 24560+8192 || info.UsedMB != 1024+512 {
		t.Fatalf("aggregate=%+v counted a device twice", info)
	}
}

func TestDedupeTreatsDomainPaddedBusIDsAsOneDevice(t *testing.T) {
	devices := dedupeDevices([]GPUMemoryDevice{
		{Vendor: GPUVendorNVIDIA, PCIBusID: "00000000:01:00.0", DedicatedTotalMB: 8192},
		{Vendor: GPUVendorNVIDIA, PCIBusID: "0000:01:00.0", DedicatedTotalMB: 8192},
	})
	if len(devices) != 1 {
		t.Fatalf("devices=%+v, want one", devices)
	}
}

func TestAggregateVRAMLeavesOutUnknownUsageAndUnifiedMemory(t *testing.T) {
	info, ok := AggregateVRAM([]GPUMemoryDevice{
		{DedicatedTotalMB: 8192, DedicatedUsedMB: 1024, UsageKnown: true},
		{DedicatedTotalMB: 16384, UsageKnown: false},
		{DedicatedTotalMB: 65536, DedicatedUsedMB: 30000, UsageKnown: true, Unified: true},
	})
	if !ok || info.TotalMB != 8192 || info.UsedMB != 1024 {
		t.Fatalf("aggregate=%+v", info)
	}
}

func TestJoinAdaptersMatchesCounterInstancesByLUIDAndDropsSoftware(t *testing.T) {
	adapters := []displayAdapter{
		{name: "Radeon RX 7900", vendorID: pciVendorAMD, luidHigh: 0, luidLow: 0xd1b2, dedicatedVideoMB: 20464},
		{name: "Arc A770", vendorID: pciVendorIntel, luidHigh: 1, luidLow: 0x10, dedicatedVideoMB: 16288},
		{name: "Microsoft Basic Render Driver", vendorID: pciVendorMicrosoft, luidLow: 0x99, software: true, dedicatedVideoMB: 0},
	}
	counters := map[string]int64{
		"luid_0x00000000_0x0000D1B2_phys_0": 3 << 30,
		"luid_0x00000000_0x0000d1b2_phys_1": 1 << 30,
		"luid_0x00000000_0x00000099_phys_0": 1 << 20,
		"not-an-adapter":                    5,
	}
	usage := map[string]adapterMemoryUsage{}
	for key, bytes := range sumCounterInstancesByLUID(counters) {
		usage[key] = adapterMemoryUsage{dedicatedBytes: bytes}
	}

	devices := joinAdaptersWithUsage(adapters, usage)
	if len(devices) != 2 {
		t.Fatalf("devices=%+v, want AMD and Intel only", devices)
	}
	if devices[0].Vendor != GPUVendorAMD || devices[0].DedicatedUsedMB != 4096 || !devices[0].UsageKnown || devices[0].Source != dxgiPDHDeviceSource {
		t.Fatalf("AMD adapter=%+v, want phys entries summed to 4096 MB", devices[0])
	}
	if devices[1].Vendor != GPUVendorIntel || devices[1].UsageKnown || devices[1].Source != dxgiDeviceSource {
		t.Fatalf("Intel adapter without counters=%+v, want usage unknown", devices[1])
	}
}
