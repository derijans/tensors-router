//go:build darwin

package hardware

import "context"

const unifiedMemorySource = "unified-memory"

type unifiedMemoryProbe struct{}

func (unifiedMemoryProbe) devices(ctx context.Context) ([]GPUMemoryDevice, error) {
	ram, ok := readSystemRAM(ctx)
	if !ok {
		return nil, errProbeUnavailable
	}
	return []GPUMemoryDevice{{
		Vendor:           GPUVendorApple,
		Name:             "Apple unified memory",
		Source:           unifiedMemorySource,
		DedicatedUsedMB:  ram.UsedMB,
		DedicatedTotalMB: ram.TotalMB,
		UsageKnown:       true,
		Unified:          true,
	}}, nil
}

func platformDeviceProbes(Detector) []deviceProbe {
	return []deviceProbe{unifiedMemoryProbe{}}
}
