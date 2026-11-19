package hardware

import (
	"context"
	"strconv"
	"strings"
)

const (
	GPUVendorNVIDIA  = "nvidia"
	GPUVendorAMD     = "amd"
	GPUVendorIntel   = "intel"
	GPUVendorApple   = "apple"
	GPUVendorVirtual = "virtual"
	GPUVendorOther   = "other"
)

type GPUMemoryDevice struct {
	Vendor           string `json:"vendor"`
	Name             string `json:"name"`
	PCIBusID         string `json:"pci_bus_id,omitempty"`
	LUID             string `json:"luid,omitempty"`
	Source           string `json:"source"`
	DedicatedUsedMB  int64  `json:"dedicated_used_mb"`
	DedicatedTotalMB int64  `json:"dedicated_total_mb"`
	SharedUsedMB     int64  `json:"shared_used_mb,omitempty"`
	UsageKnown       bool   `json:"usage_known"`
	Unified          bool   `json:"unified,omitempty"`
}

type DeviceMemorySource interface {
	DeviceMemory(context.Context) []GPUMemoryDevice
}

type deviceProbe interface {
	devices(context.Context) ([]GPUMemoryDevice, error)
}

type DeviceMemoryReader struct {
	probes []deviceProbe
}

func NewDeviceMemoryReader() *DeviceMemoryReader {
	return &DeviceMemoryReader{probes: platformDeviceProbes(defaultDetector())}
}

func (reader *DeviceMemoryReader) DeviceMemory(ctx context.Context) []GPUMemoryDevice {
	return mergeProbeResults(ctx, reader.probes)
}

func (reader *DeviceMemoryReader) VRAM(ctx context.Context) (VRAMInfo, bool) {
	return AggregateVRAM(reader.DeviceMemory(ctx))
}

func mergeProbeResults(ctx context.Context, probes []deviceProbe) []GPUMemoryDevice {
	var merged []GPUMemoryDevice
	ownedVendors := map[string]struct{}{}
	for _, probe := range probes {
		found, err := probe.devices(ctx)
		if err != nil || len(found) == 0 {
			continue
		}
		reportedVendors := map[string]struct{}{}
		for _, device := range found {
			if _, owned := ownedVendors[device.Vendor]; owned {
				continue
			}
			merged = append(merged, device)
			reportedVendors[device.Vendor] = struct{}{}
		}
		for vendor := range reportedVendors {
			ownedVendors[vendor] = struct{}{}
		}
	}
	return dedupeDevices(merged)
}

func dedupeDevices(devices []GPUMemoryDevice) []GPUMemoryDevice {
	seen := map[string]struct{}{}
	unique := make([]GPUMemoryDevice, 0, len(devices))
	for _, device := range devices {
		identity := device.identity()
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		unique = append(unique, device)
	}
	return unique
}

func (device GPUMemoryDevice) identity() string {
	if busID := normalizedPCIBusID(device.PCIBusID); busID != "" {
		return "pci:" + busID
	}
	if device.LUID != "" {
		return "luid:" + device.LUID
	}
	return "name:" + device.Vendor + "|" + device.Name + "|" + strconv.FormatInt(device.DedicatedTotalMB, 10)
}

func normalizedPCIBusID(busID string) string {
	busID = strings.ToLower(strings.TrimSpace(busID))
	if parts := strings.Split(busID, ":"); len(parts) == 3 && len(parts[0]) > 4 {
		parts[0] = parts[0][len(parts[0])-4:]
		busID = strings.Join(parts, ":")
	}
	return busID
}

func AggregateVRAM(devices []GPUMemoryDevice) (VRAMInfo, bool) {
	var usedMB, totalMB int64
	for _, device := range devices {
		if !device.UsageKnown || device.Unified || device.DedicatedTotalMB <= 0 {
			continue
		}
		usedMB += device.DedicatedUsedMB
		totalMB += device.DedicatedTotalMB
	}
	return vramInfo(usedMB, totalMB)
}
