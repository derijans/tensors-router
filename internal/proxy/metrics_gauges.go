package proxy

import (
	"context"
	"sort"
	"strconv"

	"tensors-router/internal/hardware"
	"tensors-router/internal/metrics"
)

const bytesPerMB = 1024 * 1024

type metricsGaugeDeps interface {
	localRuntimes() []*backendRuntime
	transportBudgetUsage() (used int64, limit int64)
	offloadQueueStatsByLane() map[string][]offloadModelStats
	peerHealth() map[string]bool
	nodeMemorySample(ctx context.Context) (hardware.MemoryReading, bool)
	nodeMemorySnapshot(ctx context.Context) (hardware.NodeMemorySnapshot, bool)
}

type runtimeGaugeRow struct {
	labels         []metrics.Label
	modelID        string
	users          int
	queuedWaiters  int
	memoryLoadedMB int64
}

func runtimeGaugeRowOf(runtime *backendRuntime) runtimeGaugeRow {
	state := runtime.state
	state.mu.Lock()
	defer state.mu.Unlock()
	return runtimeGaugeRow{
		labels:         []metrics.Label{{Name: "runtime", Value: runtime.name}, {Name: "backend_mode", Value: runtime.mode}},
		modelID:        state.modelID,
		users:          state.users,
		queuedWaiters:  state.queuedWaitersLocked(),
		memoryLoadedMB: state.memoryLoadedMB,
	}
}

func collectGauges(ctx context.Context, deps metricsGaugeDeps) []metrics.GaugeFamily {
	gauges := runtimeGauges(deps.localRuntimes())
	gauges = append(gauges, offloadQueueGauges(deps.offloadQueueStatsByLane())...)
	used, limit := deps.transportBudgetUsage()
	gauges = append(gauges,
		singleGauge("transport_budget_used_bytes", "Bytes of the transport memory budget reserved by requests in flight.", float64(used)),
		singleGauge("transport_budget_limit_bytes", "Size of the transport memory budget.", float64(limit)),
	)
	gauges = append(gauges, peerHealthGauge(deps.peerHealth()))
	if reading, ok := deps.nodeMemorySample(ctx); ok {
		kind := []metrics.Label{{Name: "kind", Value: reading.Kind}}
		gauges = append(gauges,
			metrics.GaugeFamily{Name: metricsPrefix + "node_memory_used_bytes", Help: "Memory in use on this node, from the memory source the router schedules with.", Samples: []metrics.GaugeSample{{Labels: kind, Value: float64(reading.UsedMB) * bytesPerMB}}},
			metrics.GaugeFamily{Name: metricsPrefix + "node_memory_total_bytes", Help: "Memory available on this node, from the memory source the router schedules with.", Samples: []metrics.GaugeSample{{Labels: kind, Value: float64(reading.TotalMB) * bytesPerMB}}},
		)
	}
	if snapshot, ok := deps.nodeMemorySnapshot(ctx); ok {
		gauges = append(gauges, deviceMemoryGauges(snapshot)...)
	}
	return gauges
}

func deviceMemoryGauges(snapshot hardware.NodeMemorySnapshot) []metrics.GaugeFamily {
	used := metrics.GaugeFamily{Name: metricsPrefix + "gpu_memory_dedicated_used_bytes", Help: "Dedicated memory in use on each GPU."}
	total := metrics.GaugeFamily{Name: metricsPrefix + "gpu_memory_dedicated_total_bytes", Help: "Dedicated memory on each GPU."}
	shared := metrics.GaugeFamily{Name: metricsPrefix + "gpu_memory_shared_used_bytes", Help: "System memory each GPU has mapped as shared memory."}
	for index, device := range snapshot.Devices {
		labels := []metrics.Label{{Name: "device", Value: "gpu" + strconv.Itoa(index)}, {Name: "vendor", Value: device.Vendor}, {Name: "name", Value: device.Name}}
		total.Samples = append(total.Samples, metrics.GaugeSample{Labels: labels, Value: float64(device.DedicatedTotalMB) * bytesPerMB})
		if !device.UsageKnown {
			continue
		}
		used.Samples = append(used.Samples, metrics.GaugeSample{Labels: labels, Value: float64(device.DedicatedUsedMB) * bytesPerMB})
		shared.Samples = append(shared.Samples, metrics.GaugeSample{Labels: labels, Value: float64(device.SharedUsedMB) * bytesPerMB})
	}
	families := []metrics.GaugeFamily{used, total, shared}
	if snapshot.RAMKnown {
		families = append(families,
			singleGauge("system_memory_used_bytes", "System RAM in use on this node.", float64(snapshot.RAM.UsedMB)*bytesPerMB),
			singleGauge("system_memory_total_bytes", "System RAM on this node.", float64(snapshot.RAM.TotalMB)*bytesPerMB),
		)
	}
	return families
}

func runtimeGauges(runtimes []*backendRuntime) []metrics.GaugeFamily {
	users := metrics.GaugeFamily{Name: metricsPrefix + "runtime_users", Help: "Requests holding each backend runtime."}
	waiters := metrics.GaugeFamily{Name: metricsPrefix + "runtime_queue_waiters", Help: "Requests waiting for each backend runtime."}
	loaded := metrics.GaugeFamily{Name: metricsPrefix + "runtime_loaded", Help: "1 for the model each backend runtime currently holds."}
	memory := metrics.GaugeFamily{Name: metricsPrefix + "runtime_memory_estimate_bytes", Help: "Memory the last load of each runtime added on this node."}
	for _, runtime := range runtimes {
		row := runtimeGaugeRowOf(runtime)
		users.Samples = append(users.Samples, metrics.GaugeSample{Labels: row.labels, Value: float64(row.users)})
		waiters.Samples = append(waiters.Samples, metrics.GaugeSample{Labels: row.labels, Value: float64(row.queuedWaiters)})
		if row.modelID == "" {
			continue
		}
		loaded.Samples = append(loaded.Samples, metrics.GaugeSample{Labels: append(append([]metrics.Label(nil), row.labels...), metrics.Label{Name: "model", Value: row.modelID}), Value: 1})
		memory.Samples = append(memory.Samples, metrics.GaugeSample{Labels: row.labels, Value: float64(row.memoryLoadedMB) * bytesPerMB})
	}
	return []metrics.GaugeFamily{users, waiters, loaded, memory}
}

func offloadQueueGauges(statsByLane map[string][]offloadModelStats) []metrics.GaugeFamily {
	pending := metrics.GaugeFamily{Name: metricsPrefix + "offload_queue_pending", Help: "Queued requests that may still be lent to another node."}
	backlog := metrics.GaugeFamily{Name: metricsPrefix + "offload_queue_backlog", Help: "Queued and admitted requests owned by this node."}
	lanes := make([]string, 0, len(statsByLane))
	for lane := range statsByLane {
		lanes = append(lanes, lane)
	}
	sort.Strings(lanes)
	for _, lane := range lanes {
		for _, stats := range statsByLane[lane] {
			labels := []metrics.Label{{Name: "lane", Value: lane}, {Name: "model", Value: stats.ModelID}}
			pending.Samples = append(pending.Samples, metrics.GaugeSample{Labels: labels, Value: float64(stats.PendingCount)})
			backlog.Samples = append(backlog.Samples, metrics.GaugeSample{Labels: labels, Value: float64(stats.BacklogCount)})
		}
	}
	return []metrics.GaugeFamily{pending, backlog}
}

func peerHealthGauge(health map[string]bool) metrics.GaugeFamily {
	gauge := metrics.GaugeFamily{Name: metricsPrefix + "cluster_peer_healthy", Help: "1 when a cluster peer passed its last health probe."}
	nodeIDs := make([]string, 0, len(health))
	for nodeID := range health {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		value := 0.0
		if health[nodeID] {
			value = 1
		}
		gauge.Samples = append(gauge.Samples, metrics.GaugeSample{Labels: []metrics.Label{{Name: "node", Value: nodeID}}, Value: value})
	}
	return gauge
}

func singleGauge(name string, help string, value float64) metrics.GaugeFamily {
	return metrics.GaugeFamily{Name: metricsPrefix + name, Help: help, Samples: []metrics.GaugeSample{{Value: value}}}
}
