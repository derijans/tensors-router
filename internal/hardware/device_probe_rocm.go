package hardware

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
)

const (
	rocmSMICommand      = "rocm-smi"
	rocmSMIDeviceSource = "rocm-smi"
)

type rocmSMIProbe struct {
	detector Detector
}

func (probe rocmSMIProbe) devices(ctx context.Context) ([]GPUMemoryDevice, error) {
	if !commandExists(probe.detector, rocmSMICommand) {
		return nil, errProbeUnavailable
	}
	output, err := runWithTimeout(ctx, probe.detector, rocmSMICommand, "--showmeminfo", "vram", "--csv")
	if err != nil {
		return nil, err
	}
	return parseROCmDevices(output), nil
}

func parseROCmDevices(output []byte) []GPUMemoryDevice {
	reader := csv.NewReader(bytes.NewReader(output))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return parseROCmTextTotals(output)
	}
	columns, ok := findROCmVRAMColumns(records[0])
	if !ok {
		return parseROCmTextTotals(output)
	}
	var devices []GPUMemoryDevice
	for _, record := range records[1:] {
		if len(record) <= columns.usedIndex || len(record) <= columns.totalIndex {
			continue
		}
		used, total, ok := columns.rowVRAM(record)
		if !ok {
			continue
		}
		devices = append(devices, rocmDevice(strings.TrimSpace(record[0]), used, total))
	}
	return devices
}

func rocmDevice(name string, usedMB int64, totalMB int64) GPUMemoryDevice {
	return GPUMemoryDevice{Vendor: GPUVendorAMD, Name: name, Source: rocmSMIDeviceSource, DedicatedUsedMB: usedMB, DedicatedTotalMB: totalMB, UsageKnown: true}
}

type rocmVRAMColumns struct {
	usedIndex   int
	totalIndex  int
	usedHeader  string
	totalHeader string
}

func findROCmVRAMColumns(headers []string) (rocmVRAMColumns, bool) {
	columns := rocmVRAMColumns{usedIndex: -1, totalIndex: -1}
	for index, header := range headers {
		normalized := strings.ToLower(strings.TrimSpace(header))
		if !strings.Contains(normalized, "vram") {
			continue
		}
		if strings.Contains(normalized, "used") {
			columns.usedIndex = index
		} else if strings.Contains(normalized, "total") {
			columns.totalIndex = index
		}
	}
	if columns.usedIndex < 0 || columns.totalIndex < 0 {
		return rocmVRAMColumns{}, false
	}
	columns.usedHeader = strings.ToLower(headers[columns.usedIndex])
	columns.totalHeader = strings.ToLower(headers[columns.totalIndex])
	return columns, true
}

func (columns rocmVRAMColumns) rowVRAM(record []string) (int64, int64, bool) {
	used, ok := parseVRAMValue(record[columns.usedIndex], columns.usedHeader)
	if !ok {
		return 0, 0, false
	}
	total, ok := parseVRAMValue(record[columns.totalIndex], columns.totalHeader)
	if !ok || total <= 0 {
		return 0, 0, false
	}
	return used, total, true
}

func parseVRAMValue(value string, header string) (int64, bool) {
	parsed, ok := parseInt64Text(value)
	if !ok {
		return 0, false
	}
	if strings.Contains(header, "(b)") || strings.Contains(header, "byte") {
		return bytesToMB(parsed), true
	}
	return parsed, true
}

func parseROCmTextTotals(output []byte) []GPUMemoryDevice {
	var usedMB, totalMB int64
	for _, line := range splitOutputLines(output) {
		normalized := strings.ToLower(line)
		if !strings.Contains(normalized, "vram") {
			continue
		}
		value, ok := lastInt64InText(line)
		if !ok {
			continue
		}
		if strings.Contains(normalized, "used") {
			usedMB += bytesToMB(value)
		}
		if strings.Contains(normalized, "total") {
			totalMB += bytesToMB(value)
		}
	}
	if totalMB <= 0 {
		return nil
	}
	return []GPUMemoryDevice{rocmDevice("rocm-smi total", usedMB, totalMB)}
}
