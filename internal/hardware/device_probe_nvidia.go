package hardware

import (
	"context"
	"errors"
	"strings"
)

const nvidiaSMIDeviceSource = "nvidia-smi"

var errProbeUnavailable = errors.New("memory probe unavailable on this node")

type nvidiaSMIProbe struct {
	detector Detector
}

func (probe nvidiaSMIProbe) devices(ctx context.Context) ([]GPUMemoryDevice, error) {
	if !commandExists(probe.detector, nvidiaSMICommand) {
		return nil, errProbeUnavailable
	}
	output, err := runWithTimeout(ctx, probe.detector, nvidiaSMICommand, "--query-gpu=index,pci.bus_id,name,memory.used,memory.total", "--format=csv,noheader,nounits")
	if err != nil {
		return nil, err
	}
	return parseNvidiaDevices(output), nil
}

func parseNvidiaDevices(output []byte) []GPUMemoryDevice {
	var devices []GPUMemoryDevice
	for _, line := range splitOutputLines(output) {
		fields := strings.Split(line, ",")
		if len(fields) != 5 {
			continue
		}
		used, usedOK := parseInt64Text(fields[3])
		total, totalOK := parseInt64Text(fields[4])
		if !usedOK || !totalOK || total <= 0 || used < 0 {
			continue
		}
		devices = append(devices, GPUMemoryDevice{
			Vendor:           GPUVendorNVIDIA,
			Name:             strings.TrimSpace(fields[2]),
			PCIBusID:         strings.TrimSpace(fields[1]),
			Source:           nvidiaSMIDeviceSource,
			DedicatedUsedMB:  used,
			DedicatedTotalMB: total,
			UsageKnown:       true,
		})
	}
	return devices
}

func splitOutputLines(output []byte) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
