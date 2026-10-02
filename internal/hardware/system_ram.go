package hardware

import (
	"bufio"
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
)

const bytesPerMB = 1024 * 1024

type RAMInfo struct {
	UsedMB  int64
	TotalMB int64
}

type RAMSource interface {
	RAM(context.Context) (RAMInfo, bool)
}

type SystemRAMReader struct{}

func (SystemRAMReader) RAM(ctx context.Context) (RAMInfo, bool) {
	return readSystemRAM(ctx)
}

func ramFromBytes(totalBytes uint64, usedBytes uint64) RAMInfo {
	return RAMInfo{UsedMB: int64(usedBytes / bytesPerMB), TotalMB: int64(totalBytes / bytesPerMB)}
}

func parseMeminfo(data []byte) (RAMInfo, bool) {
	kilobytesByKey := map[string]uint64{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, rest, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		if value, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
			kilobytesByKey[key] = value
		}
	}
	total, hasTotal := kilobytesByKey["MemTotal"]
	available, hasAvailable := kilobytesByKey["MemAvailable"]
	if !hasTotal || !hasAvailable || total == 0 || available > total {
		return RAMInfo{}, false
	}
	return ramFromBytes(total*1024, (total-available)*1024), true
}

var vmStatPageSizePattern = regexp.MustCompile(`page size of (\d+) bytes`)

func parseVMStat(output []byte, totalBytes uint64) (RAMInfo, bool) {
	pageSizeMatch := vmStatPageSizePattern.FindSubmatch(output)
	if pageSizeMatch == nil || totalBytes == 0 {
		return RAMInfo{}, false
	}
	pageSize, err := strconv.ParseUint(string(pageSizeMatch[1]), 10, 64)
	if err != nil || pageSize == 0 {
		return RAMInfo{}, false
	}
	reclaimablePages := uint64(0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		key, rest, found := strings.Cut(scanner.Text(), ":")
		if !found || !isReclaimableVMStatKey(key) {
			continue
		}
		pages, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(rest), "."), 10, 64)
		if err != nil {
			return RAMInfo{}, false
		}
		reclaimablePages += pages
	}
	availableBytes := min(reclaimablePages*pageSize, totalBytes)
	return ramFromBytes(totalBytes, totalBytes-availableBytes), true
}

func isReclaimableVMStatKey(key string) bool {
	switch key {
	case "Pages free", "Pages inactive", "Pages speculative":
		return true
	default:
		return false
	}
}
