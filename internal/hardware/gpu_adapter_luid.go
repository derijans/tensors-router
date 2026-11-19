package hardware

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	dxgiPDHDeviceSource = "dxgi+pdh"
	dxgiDeviceSource    = "dxgi"
)

type displayAdapter struct {
	name             string
	vendorID         uint32
	luidHigh         int32
	luidLow          uint32
	dedicatedVideoMB int64
	software         bool
}

type adapterMemoryUsage struct {
	dedicatedBytes int64
	sharedBytes    int64
}

func adapterLUIDKey(high int32, low uint32) string {
	return fmt.Sprintf("0x%08x_0x%08x", uint32(high), low)
}

func luidKeyFromCounterInstance(instance string) (string, bool) {
	rest, found := strings.CutPrefix(strings.ToLower(strings.TrimSpace(instance)), "luid_")
	if !found {
		return "", false
	}
	parts := strings.Split(rest, "_")
	if len(parts) < 2 {
		return "", false
	}
	high, highErr := strconv.ParseUint(strings.TrimPrefix(parts[0], "0x"), 16, 32)
	low, lowErr := strconv.ParseUint(strings.TrimPrefix(parts[1], "0x"), 16, 32)
	if highErr != nil || lowErr != nil {
		return "", false
	}
	return adapterLUIDKey(int32(uint32(high)), uint32(low)), true
}

func joinAdaptersWithUsage(adapters []displayAdapter, usageByLUID map[string]adapterMemoryUsage) []GPUMemoryDevice {
	devices := make([]GPUMemoryDevice, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter.software || adapter.dedicatedVideoMB <= 0 {
			continue
		}
		key := adapterLUIDKey(adapter.luidHigh, adapter.luidLow)
		usage, usageKnown := usageByLUID[key]
		source := dxgiDeviceSource
		if usageKnown {
			source = dxgiPDHDeviceSource
		}
		devices = append(devices, GPUMemoryDevice{
			Vendor:           vendorFromPCIID(adapter.vendorID),
			Name:             adapter.name,
			LUID:             key,
			Source:           source,
			DedicatedUsedMB:  bytesToMB(usage.dedicatedBytes),
			DedicatedTotalMB: adapter.dedicatedVideoMB,
			SharedUsedMB:     bytesToMB(usage.sharedBytes),
			UsageKnown:       usageKnown,
		})
	}
	return devices
}

func sumCounterInstancesByLUID(values map[string]int64) map[string]int64 {
	sums := map[string]int64{}
	for instance, value := range values {
		if key, ok := luidKeyFromCounterInstance(instance); ok && value > 0 {
			sums[key] += value
		}
	}
	return sums
}
