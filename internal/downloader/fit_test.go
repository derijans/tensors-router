package downloader

import "testing"

func TestEstimateHardwareFitUsesPhysicalVRAMAndSplitCapability(t *testing.T) {
	request := FitRequest{
		BackendKnown:         true,
		WeightBytes:          7 << 30,
		KVBytesPerToken:      1 << 20,
		RuntimeOverheadBytes: 1 << 30,
		RequestedContext:     8192,
		DeclaredMaxContext:   32768,
		Devices: []DeviceCapability{
			{TotalVRAMBytes: 12 << 30, SplitOffloadSupported: true},
			{TotalVRAMBytes: 12 << 30, SplitOffloadSupported: true},
		},
		VRAMReserveBytes: 0,
	}
	fit := EstimateHardwareFit(request)
	if fit.Status != "partial-offload" || fit.LargestSingleGPUBytes != 12<<30 || fit.SplitGPUBytes != 24<<30 {
		t.Fatalf("unexpected split fit %#v", fit)
	}
	request.Devices[1].SplitOffloadSupported = false
	fit = EstimateHardwareFit(request)
	if fit.Status != "does-not-fit" {
		t.Fatalf("expected unsupported split result %#v", fit)
	}
	request.BackendKnown = false
	fit = EstimateHardwareFit(request)
	if fit.Status != "unknown" || fit.BackendSupported != "unknown" {
		t.Fatalf("expected unknown backend result %#v", fit)
	}
}
