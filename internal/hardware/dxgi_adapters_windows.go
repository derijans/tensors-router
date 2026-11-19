//go:build windows

package hardware

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dxgiErrorNotFound       = 0x887A0002
	dxgiAdapterFlagSoftware = 2
	dxgiMaxAdapters         = 64

	vtableRelease       = 2
	vtableEnumAdapters1 = 12
	vtableGetDesc1      = 10
)

var (
	dxgiLibrary            = windows.NewLazySystemDLL("dxgi.dll")
	createDXGIFactory1Proc = dxgiLibrary.NewProc("CreateDXGIFactory1")
	iidIDXGIFactory1       = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

type dxgiAdapterDesc1 struct {
	description           [128]uint16
	vendorID              uint32
	deviceID              uint32
	subSysID              uint32
	revision              uint32
	dedicatedVideoMemory  uintptr
	dedicatedSystemMemory uintptr
	sharedSystemMemory    uintptr
	adapterLUIDLow        uint32
	adapterLUIDHigh       int32
	flags                 uint32
}

func enumerateDisplayAdapters() ([]displayAdapter, error) {
	if err := createDXGIFactory1Proc.Find(); err != nil {
		return nil, err
	}
	var factory unsafe.Pointer
	result, _, _ := createDXGIFactory1Proc.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory)))
	if failedHRESULT(result) || factory == nil {
		return nil, errors.New("CreateDXGIFactory1 failed")
	}
	defer releaseCOMObject(factory)
	var adapters []displayAdapter
	for index := uintptr(0); index < dxgiMaxAdapters; index++ {
		var adapter unsafe.Pointer
		result := callCOMMethod(factory, vtableEnumAdapters1, index, uintptr(unsafe.Pointer(&adapter)))
		if uint32(result) == dxgiErrorNotFound {
			break
		}
		if failedHRESULT(result) || adapter == nil {
			continue
		}
		if described, ok := describeAdapter(adapter); ok {
			adapters = append(adapters, described)
		}
		releaseCOMObject(adapter)
	}
	return adapters, nil
}

func describeAdapter(adapter unsafe.Pointer) (displayAdapter, bool) {
	var desc dxgiAdapterDesc1
	if failedHRESULT(callCOMMethod(adapter, vtableGetDesc1, uintptr(unsafe.Pointer(&desc)))) {
		return displayAdapter{}, false
	}
	return displayAdapter{
		name:             windows.UTF16ToString(desc.description[:]),
		vendorID:         desc.vendorID,
		luidHigh:         desc.adapterLUIDHigh,
		luidLow:          desc.adapterLUIDLow,
		dedicatedVideoMB: bytesToMB(int64(desc.dedicatedVideoMemory)),
		software:         desc.flags&dxgiAdapterFlagSoftware != 0,
	}, true
}

func callCOMMethod(object unsafe.Pointer, slot int, arguments ...uintptr) uintptr {
	methods := unsafe.Slice(*(**uintptr)(object), slot+1)
	result, _, _ := syscall.SyscallN(methods[slot], append([]uintptr{uintptr(object)}, arguments...)...)
	return result
}

func releaseCOMObject(object unsafe.Pointer) {
	callCOMMethod(object, vtableRelease)
}

func failedHRESULT(result uintptr) bool {
	return int32(uint32(result)) < 0
}
