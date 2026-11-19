package proxy

import (
	"context"

	"tensors-router/internal/hardware"
	"tensors-router/internal/siteapi"
)

func (service *Service) nodeMemoryView(ctx context.Context) *siteapi.NodeMemory {
	if service.nodeMemory == nil {
		return nil
	}
	reading, ok := service.nodeMemory.Memory(ctx)
	if !ok {
		return nil
	}
	view := &siteapi.NodeMemory{Kind: reading.Kind, TotalMB: reading.TotalMB, UsedMB: reading.UsedMB, SampledAtMS: reading.SampledAt.UnixMilli()}
	if source, lists := service.nodeMemory.(hardware.NodeMemorySnapshotSource); lists {
		if snapshot, known := source.NodeMemory(ctx); known {
			view.Devices = nodeMemoryDevices(snapshot.Devices)
			if snapshot.RAMKnown {
				view.SystemRAM = &siteapi.NodeSystemRAM{UsedMB: snapshot.RAM.UsedMB, TotalMB: snapshot.RAM.TotalMB}
			}
		}
	}
	return view
}

func nodeMemoryDevices(devices []hardware.GPUMemoryDevice) []siteapi.NodeMemoryDevice {
	if len(devices) == 0 {
		return nil
	}
	views := make([]siteapi.NodeMemoryDevice, len(devices))
	for index, device := range devices {
		views[index] = siteapi.NodeMemoryDevice{
			Vendor:           device.Vendor,
			Name:             device.Name,
			Source:           device.Source,
			DedicatedUsedMB:  device.DedicatedUsedMB,
			DedicatedTotalMB: device.DedicatedTotalMB,
			SharedUsedMB:     device.SharedUsedMB,
			UsageKnown:       device.UsageKnown,
			Unified:          device.Unified,
		}
	}
	return views
}
