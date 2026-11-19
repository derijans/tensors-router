package hardware

import (
	"context"
	"strconv"
	"strings"
)

type VRAMInfo struct {
	UsedMB      int64   `json:"used_mb"`
	TotalMB     int64   `json:"total_mb"`
	UsedPercent float64 `json:"used_percent"`
}

type VRAMSource interface {
	VRAM(context.Context) (VRAMInfo, bool)
}

type VRAMReader struct {
	devices *DeviceMemoryReader
}

func NewVRAMReader() *VRAMReader {
	return &VRAMReader{devices: NewDeviceMemoryReader()}
}

func (reader *VRAMReader) VRAM(ctx context.Context) (VRAMInfo, bool) {
	if reader == nil || reader.devices == nil {
		return NewDeviceMemoryReader().VRAM(ctx)
	}
	return reader.devices.VRAM(ctx)
}

func (reader *VRAMReader) DeviceMemory(ctx context.Context) []GPUMemoryDevice {
	if reader == nil || reader.devices == nil {
		return NewDeviceMemoryReader().DeviceMemory(ctx)
	}
	return reader.devices.DeviceMemory(ctx)
}

func ReadVRAM(ctx context.Context, detector Detector) (VRAMInfo, bool) {
	if detector.LookPath == nil || detector.Run == nil {
		detector = defaultDetector()
	}
	return AggregateVRAM(mergeProbeResults(ctx, vendorToolProbes(detector)))
}

func vendorToolProbes(detector Detector) []deviceProbe {
	return []deviceProbe{nvidiaSMIProbe{detector: detector}, rocmSMIProbe{detector: detector}}
}

func parseInt64Text(value string) (int64, bool) {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.Trim(cleaned, `"`)
	parsed, err := strconv.ParseInt(cleaned, 10, 64)
	return parsed, err == nil
}

func lastInt64InText(value string) (int64, bool) {
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return char < '0' || char > '9'
	})
	for index := len(parts) - 1; index >= 0; index-- {
		if parts[index] == "" {
			continue
		}
		if parsed, ok := parseInt64Text(parts[index]); ok {
			return parsed, true
		}
	}
	return 0, false
}

func bytesToMB(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return (value + 512*1024) / (1024 * 1024)
}

func vramInfo(usedMB int64, totalMB int64) (VRAMInfo, bool) {
	if totalMB <= 0 {
		return VRAMInfo{}, false
	}
	if usedMB < 0 {
		usedMB = 0
	}
	return VRAMInfo{
		UsedMB:      usedMB,
		TotalMB:     totalMB,
		UsedPercent: float64(usedMB) / float64(totalMB) * 100,
	}, true
}
