package downloader

import "sort"

type FitRequest struct {
	BackendKnown         bool
	WeightBytes          int64
	KVBytesPerToken      int64
	RuntimeOverheadBytes int64
	RequestedContext     int
	DeclaredMaxContext   int
	Devices              []DeviceCapability
	VRAMReserveBytes     int64
	SafetyMarginPercent  int
}

func EstimateHardwareFit(request FitRequest) HardwareFit {
	result := HardwareFit{Status: "unknown", BackendSupported: "unknown", EstimatedWeightBytes: request.WeightBytes, EstimatedKVBytes: request.KVBytesPerToken * int64(request.RequestedContext), EstimatedOverheadBytes: request.RuntimeOverheadBytes, RequestedContext: request.RequestedContext}
	if request.DeclaredMaxContext > 0 && (result.RequestedContext == 0 || result.RequestedContext > request.DeclaredMaxContext) {
		result.RequestedContext = request.DeclaredMaxContext
		result.EstimatedKVBytes = request.KVBytesPerToken * int64(result.RequestedContext)
	}
	if !request.BackendKnown || request.WeightBytes < 0 || request.KVBytesPerToken < 0 || request.RuntimeOverheadBytes < 0 || request.RequestedContext < 0 || request.SafetyMarginPercent < 0 || request.SafetyMarginPercent >= 100 {
		result.Reason = "backend or artifact metadata is unknown"
		return result
	}
	result.BackendSupported = "supported"
	capacities := make([]int64, 0, len(request.Devices))
	splitSupported := true
	for _, device := range request.Devices {
		if device.TotalVRAMBytes <= 0 {
			continue
		}
		capacity := device.TotalVRAMBytes - request.VRAMReserveBytes
		capacity -= capacity * int64(request.SafetyMarginPercent) / 100
		if capacity < 0 {
			capacity = 0
		}
		capacities = append(capacities, capacity)
		splitSupported = splitSupported && device.SplitOffloadSupported
	}
	if len(capacities) == 0 {
		result.Reason = "selected node did not report physical GPU VRAM"
		return result
	}
	if len(capacities) < 2 {
		splitSupported = false
	}
	sort.Slice(capacities, func(left int, right int) bool { return capacities[left] > capacities[right] })
	result.LargestSingleGPUBytes = capacities[0]
	for _, capacity := range capacities {
		result.SplitGPUBytes += capacity
	}
	required := request.WeightBytes + result.EstimatedKVBytes + request.RuntimeOverheadBytes
	result.MaximumContext = maximumContext(capacities[0], request.WeightBytes, request.RuntimeOverheadBytes, request.KVBytesPerToken, request.DeclaredMaxContext)
	if required <= capacities[0] {
		result.Status = "fits"
		return result
	}
	if len(capacities) > 1 && splitSupported && required <= result.SplitGPUBytes {
		result.Status = "partial-offload"
		return result
	}
	result.Status = "does-not-fit"
	return result
}

func maximumContext(capacity int64, weight int64, overhead int64, kvPerToken int64, declared int) int {
	if kvPerToken <= 0 {
		return declared
	}
	available := capacity - weight - overhead
	if available <= 0 {
		return 0
	}
	maximum := int(available / kvPerToken)
	if declared > 0 && maximum > declared {
		return declared
	}
	return maximum
}
