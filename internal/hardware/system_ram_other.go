//go:build !linux && !darwin && !windows

package hardware

import "context"

func readSystemRAM(context.Context) (RAMInfo, bool) {
	return RAMInfo{}, false
}
