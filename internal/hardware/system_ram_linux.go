//go:build linux

package hardware

import (
	"context"
	"os"
)

func readSystemRAM(context.Context) (RAMInfo, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return RAMInfo{}, false
	}
	return parseMeminfo(data)
}
