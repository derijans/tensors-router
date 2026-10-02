//go:build windows

package hardware

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

var globalMemoryStatusProcedure = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

type memoryStatusEx struct {
	length    uint32
	_         uint32
	totalPhys uint64
	availPhys uint64
	_         [5]uint64
}

func readSystemRAM(context.Context) (RAMInfo, bool) {
	status := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, _ := globalMemoryStatusProcedure.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 || status.totalPhys == 0 || status.availPhys > status.totalPhys {
		return RAMInfo{}, false
	}
	return ramFromBytes(status.totalPhys, status.totalPhys-status.availPhys), true
}
