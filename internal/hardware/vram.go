package hardware

import (
	"bytes"
	"context"
	"encoding/csv"
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
	detector Detector
}

func NewVRAMReader() *VRAMReader {
	return &VRAMReader{detector: defaultDetector()}
}

func NewVRAMReaderWithDetector(detector Detector) *VRAMReader {
	return &VRAMReader{detector: detector}
}

func (reader *VRAMReader) VRAM(ctx context.Context) (VRAMInfo, bool) {
	if reader == nil {
		return ReadVRAM(ctx, defaultDetector())
	}
	return ReadVRAM(ctx, reader.detector)
}

func ReadVRAM(ctx context.Context, detector Detector) (VRAMInfo, bool) {
	if detector.LookPath == nil || detector.Run == nil {
		detector = defaultDetector()
	}
	if commandExists(detector, "nvidia-smi") {
		output, err := runWithTimeout(ctx, detector, "nvidia-smi", "--query-gpu=memory.used,memory.total", "--format=csv,noheader,nounits")
		if err == nil {
			if info, ok := parseNvidiaVRAM(output); ok {
				return info, true
			}
		}
	}
	if commandExists(detector, "rocm-smi") {
		output, err := runWithTimeout(ctx, detector, "rocm-smi", "--showmeminfo", "vram", "--csv")
		if err == nil {
			if info, ok := parseROCmVRAM(output); ok {
				return info, true
			}
		}
	}
	return VRAMInfo{}, false
}

func parseNvidiaVRAM(output []byte) (VRAMInfo, bool) {
	rows := bytes.Split(bytes.TrimSpace(output), []byte{'\n'})
	var usedMB int64
	var totalMB int64
	for _, row := range rows {
		row = bytes.TrimSpace(row)
		if len(row) == 0 {
			continue
		}
		parts := strings.Split(string(row), ",")
		if len(parts) != 2 {
			return VRAMInfo{}, false
		}
		used, ok := parseInt64Text(parts[0])
		if !ok {
			return VRAMInfo{}, false
		}
		total, ok := parseInt64Text(parts[1])
		if !ok || total <= 0 {
			return VRAMInfo{}, false
		}
		usedMB += used
		totalMB += total
	}
	return vramInfo(usedMB, totalMB)
}

func parseROCmVRAM(output []byte) (VRAMInfo, bool) {
	reader := csv.NewReader(bytes.NewReader(output))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err == nil {
		if info, ok := parseROCmCSV(records); ok {
			return info, true
		}
	}
	return parseROCmText(output)
}

func parseROCmCSV(records [][]string) (VRAMInfo, bool) {
	if len(records) < 2 {
		return VRAMInfo{}, false
	}
	usedIndex := -1
	totalIndex := -1
	headers := records[0]
	for index, header := range headers {
		normalized := strings.ToLower(strings.TrimSpace(header))
		if strings.Contains(normalized, "vram") && strings.Contains(normalized, "used") {
			usedIndex = index
		}
		if strings.Contains(normalized, "vram") && strings.Contains(normalized, "total") && !strings.Contains(normalized, "used") {
			totalIndex = index
		}
	}
	if usedIndex < 0 || totalIndex < 0 {
		return VRAMInfo{}, false
	}
	usedHeader := strings.ToLower(headers[usedIndex])
	totalHeader := strings.ToLower(headers[totalIndex])
	var usedMB int64
	var totalMB int64
	for _, record := range records[1:] {
		if len(record) <= usedIndex || len(record) <= totalIndex {
			continue
		}
		used, ok := parseVRAMValue(record[usedIndex], usedHeader)
		if !ok {
			return VRAMInfo{}, false
		}
		total, ok := parseVRAMValue(record[totalIndex], totalHeader)
		if !ok || total <= 0 {
			return VRAMInfo{}, false
		}
		usedMB += used
		totalMB += total
	}
	return vramInfo(usedMB, totalMB)
}

func parseROCmText(output []byte) (VRAMInfo, bool) {
	lines := strings.Split(string(output), "\n")
	var usedMB int64
	var totalMB int64
	for _, line := range lines {
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
	return vramInfo(usedMB, totalMB)
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
