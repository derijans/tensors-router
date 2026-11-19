//go:build windows

package hardware

import (
	"errors"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pdhMoreData            = 0x800007D2
	pdhFormatLarge         = 0x00000400
	pdhCounterArrayLimit   = 1 << 20
	gpuDedicatedUsagePath  = `\GPU Adapter Memory(*)\Dedicated Usage`
	gpuSharedUsagePath     = `\GPU Adapter Memory(*)\Shared Usage`
	pdhCounterStatusValid  = 0
	pdhCounterStatusNewSet = 1
)

var (
	pdhLibrary                       = windows.NewLazySystemDLL("pdh.dll")
	pdhOpenQueryProc                 = pdhLibrary.NewProc("PdhOpenQueryW")
	pdhAddEnglishCounterProc         = pdhLibrary.NewProc("PdhAddEnglishCounterW")
	pdhCollectQueryDataProc          = pdhLibrary.NewProc("PdhCollectQueryData")
	pdhGetFormattedCounterArrayProc  = pdhLibrary.NewProc("PdhGetFormattedCounterArrayW")
	pdhCloseQueryProc                = pdhLibrary.NewProc("PdhCloseQuery")
	errGPUAdapterCountersUnavailable = errors.New("GPU adapter memory counters unavailable")
)

type pdhFormattedCounterItem struct {
	name   *uint16
	status uint32
	_      uint32
	value  int64
}

type adapterMemoryCounters struct {
	mu        sync.Mutex
	query     uintptr
	dedicated uintptr
	shared    uintptr
	opened    bool
}

func (counters *adapterMemoryCounters) usage() (map[string]adapterMemoryUsage, error) {
	counters.mu.Lock()
	defer counters.mu.Unlock()
	if err := counters.openLocked(); err != nil {
		return nil, err
	}
	if status, _, _ := pdhCollectQueryDataProc.Call(counters.query); status != 0 {
		counters.closeLocked()
		return nil, errGPUAdapterCountersUnavailable
	}
	dedicatedValues, err := formattedCounterValues(counters.dedicated)
	if err != nil {
		return nil, err
	}
	usage := map[string]adapterMemoryUsage{}
	for key, bytes := range sumCounterInstancesByLUID(dedicatedValues) {
		usage[key] = adapterMemoryUsage{dedicatedBytes: bytes}
	}
	if counters.shared == 0 {
		return usage, nil
	}
	if sharedValues, err := formattedCounterValues(counters.shared); err == nil {
		for key, bytes := range sumCounterInstancesByLUID(sharedValues) {
			entry := usage[key]
			entry.sharedBytes = bytes
			usage[key] = entry
		}
	}
	return usage, nil
}

func (counters *adapterMemoryCounters) openLocked() error {
	if counters.opened {
		return nil
	}
	if err := pdhOpenQueryProc.Find(); err != nil {
		return err
	}
	var query uintptr
	if status, _, _ := pdhOpenQueryProc.Call(0, 0, uintptr(unsafe.Pointer(&query))); status != 0 {
		return errGPUAdapterCountersUnavailable
	}
	dedicated, err := addEnglishCounter(query, gpuDedicatedUsagePath)
	if err != nil {
		_, _, _ = pdhCloseQueryProc.Call(query)
		return err
	}
	shared, sharedErr := addEnglishCounter(query, gpuSharedUsagePath)
	if sharedErr != nil {
		shared = 0
	}
	counters.query, counters.dedicated, counters.shared, counters.opened = query, dedicated, shared, true
	return nil
}

func (counters *adapterMemoryCounters) closeLocked() {
	if counters.opened {
		_, _, _ = pdhCloseQueryProc.Call(counters.query)
	}
	counters.query, counters.dedicated, counters.shared, counters.opened = 0, 0, 0, false
}

func addEnglishCounter(query uintptr, counterPath string) (uintptr, error) {
	pathPointer, err := windows.UTF16PtrFromString(counterPath)
	if err != nil {
		return 0, err
	}
	var counter uintptr
	if status, _, _ := pdhAddEnglishCounterProc.Call(query, uintptr(unsafe.Pointer(pathPointer)), 0, uintptr(unsafe.Pointer(&counter))); status != 0 {
		return 0, errGPUAdapterCountersUnavailable
	}
	return counter, nil
}

func formattedCounterValues(counter uintptr) (map[string]int64, error) {
	var bufferSize, itemCount uint32
	status, _, _ := pdhGetFormattedCounterArrayProc.Call(counter, pdhFormatLarge, uintptr(unsafe.Pointer(&bufferSize)), uintptr(unsafe.Pointer(&itemCount)), 0)
	if uint32(status) != pdhMoreData || bufferSize == 0 || bufferSize > pdhCounterArrayLimit {
		return map[string]int64{}, nil
	}
	buffer := make([]byte, bufferSize)
	status, _, _ = pdhGetFormattedCounterArrayProc.Call(counter, pdhFormatLarge, uintptr(unsafe.Pointer(&bufferSize)), uintptr(unsafe.Pointer(&itemCount)), uintptr(unsafe.Pointer(&buffer[0])))
	if status != 0 {
		return nil, errGPUAdapterCountersUnavailable
	}
	maxItems := uintptr(len(buffer)) / unsafe.Sizeof(pdhFormattedCounterItem{})
	items := unsafe.Slice((*pdhFormattedCounterItem)(unsafe.Pointer(&buffer[0])), min(uintptr(itemCount), maxItems))
	values := make(map[string]int64, len(items))
	for _, item := range items {
		if item.status != pdhCounterStatusValid && item.status != pdhCounterStatusNewSet {
			continue
		}
		values[windows.UTF16PtrToString(item.name)] = item.value
	}
	return values, nil
}
