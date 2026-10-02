//go:build darwin

package hardware

import (
	"context"

	"golang.org/x/sys/unix"
)

func readSystemRAM(ctx context.Context) (RAMInfo, bool) {
	totalBytes, err := unix.SysctlUint64("hw.memsize")
	if err != nil || totalBytes == 0 {
		return RAMInfo{}, false
	}
	output, err := runWithTimeout(ctx, defaultDetector(), "vm_stat")
	if err != nil {
		return RAMInfo{}, false
	}
	return parseVMStat(output, totalBytes)
}
