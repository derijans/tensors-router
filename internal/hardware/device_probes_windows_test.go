//go:build windows

package hardware

import (
	"context"
	"testing"
	"time"
	"unsafe"
)

func TestDXGIAdapterDescriptionMatchesTheWindowsLayout(t *testing.T) {
	want := uintptr(312)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 292
	}
	if got := unsafe.Sizeof(dxgiAdapterDesc1{}); got != want {
		t.Fatalf("DXGI_ADAPTER_DESC1 size=%d, want %d; GetDesc1 would overwrite memory", got, want)
	}
}

func TestPDHCounterItemMatchesTheWindowsLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout pinned for 64-bit Windows")
	}
	if got := unsafe.Sizeof(pdhFormattedCounterItem{}); got != 24 {
		t.Fatalf("PDH_FMT_COUNTERVALUE_ITEM_W size=%d, want 24", got)
	}
}

func TestWindowsAdapterProbeReadsThisHostsAdapters(t *testing.T) {
	probe := newWindowsAdapterProbe()
	devices, err := probe.devices(context.Background())
	if err != nil {
		t.Fatalf("adapter probe failed: %v", err)
	}
	for _, device := range devices {
		t.Logf("%+v", device)
		if device.DedicatedTotalMB <= 0 || device.LUID == "" {
			t.Fatalf("adapter reported without total or LUID: %+v", device)
		}
		if device.UsageKnown && device.DedicatedUsedMB > device.DedicatedTotalMB*4 {
			t.Fatalf("usage looks unit-confused: %+v", device)
		}
	}
}

func TestWindowsAdapterProbeRepeatReadsStayCheap(t *testing.T) {
	probe := newWindowsAdapterProbe()
	if _, err := probe.devices(context.Background()); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	for range 5 {
		if _, err := probe.devices(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if perRead := time.Since(started) / 5; perRead > 100*time.Millisecond {
		t.Fatalf("a repeat read took %s; the 250ms VRAM sampler would fall behind", perRead)
	}
}
