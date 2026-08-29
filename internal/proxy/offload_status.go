package proxy

import (
	"context"
	"time"

	"tensors-router/internal/schedulingcost"
)

// applyImageSchedulingStatus publishes what the master needs to decide whether
// this node should lend or borrow: what it still has queued, whether it has room
// for work that is not its own, which image config it is holding, and the
// coefficients fitted from its own history.
func (service *Service) applyImageSchedulingStatus(status *NodeRuntimeStatus) {
	if service.imageQueue == nil {
		return
	}
	status.ImageQueue = service.imageQueue.Stats()
	status.AcceptingBorrowed = service.imageQueue.AcceptingBorrowed()
	status.ActiveImageConfig = service.activeImageConfigFilename()
	if costs := service.publishedCosts(); costs != nil {
		status.Costs = *costs
	}
}

func (service *Service) activeImageConfigFilename() string {
	runtime, err := service.runtimeForBackendMode(service.currentBackendMode(), readinessImage)
	if err != nil || runtime == nil {
		return ""
	}
	return currentRuntimeConfigFilename(runtime)
}

// publishedCosts caches the fit between refresh ticks. Fitting reads the analytics
// database, which runs on a single connection, so it must never happen on the path
// that answers a status poll.
func (service *Service) publishedCosts() *schedulingcost.NodeCosts {
	value := service.localCosts.Load()
	if value == nil {
		return nil
	}
	costs, ok := value.(*schedulingcost.NodeCosts)
	if !ok {
		return nil
	}
	return costs
}

func (service *Service) refreshLocalCosts(ctx context.Context) {
	costs := service.fitLocalCosts(ctx)
	service.localCosts.Store(&costs)
}

// StartSchedulingRefresh keeps the fitted costs current and, on a master, keeps
// offload leases current with them. It returns immediately; the loop stops with
// the context.
func (service *Service) StartSchedulingRefresh(ctx context.Context) {
	if service.imageQueue == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(service.schedulingRefreshInterval)
		defer ticker.Stop()
		service.refreshLocalCosts(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				service.refreshLocalCosts(ctx)
				service.refreshOffloadPlan(ctx)
			}
		}
	}()
}
