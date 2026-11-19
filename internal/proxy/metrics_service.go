package proxy

import (
	"context"
	"net/http"

	"tensors-router/internal/auth"
	"tensors-router/internal/cluster"
	"tensors-router/internal/hardware"
	"tensors-router/internal/metrics"
	"tensors-router/internal/proxy/routing"
)

func (service *Service) metricsRoutes() []routing.Route {
	if service.metrics == nil {
		return nil
	}
	return []routing.Route{routing.Exact(http.MethodGet, auth.MetricsPath, service.handleRouterMetrics)}
}

func (service *Service) handleRouterMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(headerContentType, metrics.ContentType)
	if err := service.metrics.registry.Write(w, collectGauges(r.Context(), service)); err != nil {
		service.logger.Printf("metrics scrape failed: %v", err)
	}
}

func (service *Service) knownLocalModel(modelID string) bool {
	if service.catalog == nil {
		return false
	}
	_, found, err := service.catalog.Resolve(modelID)
	return err == nil && found
}

func (service *Service) localRuntimes() []*backendRuntime {
	bindings := service.runtimeBindings()
	runtimes := make([]*backendRuntime, 0, len(bindings))
	seen := make(map[*backendRuntime]struct{}, len(bindings))
	for _, binding := range bindings {
		if _, duplicate := seen[binding.runtime]; duplicate {
			continue
		}
		seen[binding.runtime] = struct{}{}
		runtimes = append(runtimes, binding.runtime)
	}
	return runtimes
}

func (service *Service) transportBudgetUsage() (int64, int64) {
	return service.transportBudget.Used(), service.transportBudget.Limit()
}

func (service *Service) offloadQueueStatsByLane() map[string][]offloadModelStats {
	return map[string][]offloadModelStats{
		cluster.RouteLaneText:  service.scheduler.textQueue.Stats(),
		cluster.RouteLaneImage: service.scheduler.imageQueue.Stats(),
	}
}

func (service *Service) peerHealth() map[string]bool {
	if service.registry == nil {
		return nil
	}
	return service.registry.PeerHealth()
}

func (service *Service) nodeMemorySnapshot(ctx context.Context) (hardware.NodeMemorySnapshot, bool) {
	source, lists := service.nodeMemory.(hardware.NodeMemorySnapshotSource)
	if !lists {
		return hardware.NodeMemorySnapshot{}, false
	}
	return source.NodeMemory(ctx)
}

func (service *Service) nodeMemorySample(ctx context.Context) (hardware.MemoryReading, bool) {
	if service.nodeMemory == nil {
		return hardware.MemoryReading{}, false
	}
	return service.nodeMemory.Memory(ctx)
}
