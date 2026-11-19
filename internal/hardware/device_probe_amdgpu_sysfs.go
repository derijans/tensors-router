package hardware

import (
	"context"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

const (
	amdgpuSysfsSource    = "amdgpu-sysfs"
	sysfsValueReadLimit  = 128
	sysfsUeventReadLimit = 4096
)

var drmCardEntryPattern = regexp.MustCompile(`^card[0-9]+$`)

type amdgpuSysfsProbe struct {
	sysfs fs.FS
}

func (probe amdgpuSysfsProbe) devices(context.Context) ([]GPUMemoryDevice, error) {
	if probe.sysfs == nil {
		return nil, errProbeUnavailable
	}
	entries, err := fs.ReadDir(probe.sysfs, "class/drm")
	if err != nil {
		return nil, err
	}
	var devices []GPUMemoryDevice
	for _, entry := range entries {
		if !drmCardEntryPattern.MatchString(entry.Name()) {
			continue
		}
		if device, ok := probe.cardDevice(path.Join("class/drm", entry.Name(), "device")); ok {
			devices = append(devices, device)
		}
	}
	return devices, nil
}

func (probe amdgpuSysfsProbe) cardDevice(deviceDir string) (GPUMemoryDevice, bool) {
	vendorID, ok := parsePCIVendorID(readSysfsText(probe.sysfs, path.Join(deviceDir, "vendor"), sysfsValueReadLimit))
	if !ok || vendorID != pciVendorAMD {
		return GPUMemoryDevice{}, false
	}
	totalBytes, totalOK := parseInt64Text(readSysfsText(probe.sysfs, path.Join(deviceDir, "mem_info_vram_total"), sysfsValueReadLimit))
	usedBytes, usedOK := parseInt64Text(readSysfsText(probe.sysfs, path.Join(deviceDir, "mem_info_vram_used"), sysfsValueReadLimit))
	if !totalOK || totalBytes <= 0 {
		return GPUMemoryDevice{}, false
	}
	gttUsedBytes, _ := parseInt64Text(readSysfsText(probe.sysfs, path.Join(deviceDir, "mem_info_gtt_used"), sysfsValueReadLimit))
	busID := ueventValue(readSysfsText(probe.sysfs, path.Join(deviceDir, "uevent"), sysfsUeventReadLimit), "PCI_SLOT_NAME")
	return GPUMemoryDevice{
		Vendor:           GPUVendorAMD,
		Name:             "AMD GPU " + busID,
		PCIBusID:         busID,
		Source:           amdgpuSysfsSource,
		DedicatedUsedMB:  bytesToMB(usedBytes),
		DedicatedTotalMB: bytesToMB(totalBytes),
		SharedUsedMB:     bytesToMB(gttUsedBytes),
		UsageKnown:       usedOK && usedBytes >= 0,
	}, true
}

func readSysfsText(sysfs fs.FS, name string, limit int64) string {
	file, err := sysfs.Open(name)
	if err != nil {
		return ""
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func ueventValue(uevent string, key string) string {
	for _, line := range splitOutputLines([]byte(uevent)) {
		if name, value, found := strings.Cut(line, "="); found && name == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
