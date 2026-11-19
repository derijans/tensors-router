//go:build windows

package hardware

import (
	"context"
	"sync"
	"time"
)

const displayAdapterListTTL = 30 * time.Second

type windowsAdapterProbe struct {
	counters *adapterMemoryCounters

	mu         sync.Mutex
	adapters   []displayAdapter
	listedAt   time.Time
	listingErr error
}

func newWindowsAdapterProbe() *windowsAdapterProbe {
	return &windowsAdapterProbe{counters: &adapterMemoryCounters{}}
}

func (probe *windowsAdapterProbe) devices(context.Context) ([]GPUMemoryDevice, error) {
	adapters, err := probe.displayAdapters()
	if err != nil {
		return nil, err
	}
	usage, err := probe.counters.usage()
	if err != nil {
		usage = nil
	}
	return joinAdaptersWithUsage(adapters, usage), nil
}

func (probe *windowsAdapterProbe) displayAdapters() ([]displayAdapter, error) {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if !probe.listedAt.IsZero() && time.Since(probe.listedAt) < displayAdapterListTTL {
		return probe.adapters, probe.listingErr
	}
	probe.adapters, probe.listingErr = enumerateDisplayAdapters()
	probe.listedAt = time.Now()
	return probe.adapters, probe.listingErr
}

func platformDeviceProbes(detector Detector) []deviceProbe {
	return append([]deviceProbe{newWindowsAdapterProbe()}, vendorToolProbes(detector)...)
}
