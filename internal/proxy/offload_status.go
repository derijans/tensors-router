package proxy

import (
	"context"
	"time"

	"tensors-router/internal/schedulingcost"
)

func (service *Service) applyImageSchedulingStatus(status *NodeRuntimeStatus) {
	if service.imageQueue == nil {
		return
	}
	status.ImageQueue = service.imageQueue.Stats()
	status.AcceptingBorrowedImage = service.imageQueue.AcceptingBorrowed(service.nodeActivity())
	status.ActiveImageConfig = service.activeImageConfigFilename()
}

func (service *Service) applyTextSchedulingStatus(status *NodeRuntimeStatus) {
	if service.textQueue == nil {
		return
	}
	status.TextQueue = service.textQueue.Stats()
	status.AcceptingBorrowedText = service.textQueue.AcceptingBorrowed(service.nodeActivity())
	status.ActiveTextConfig = service.activeTextConfigFilename()
}

func (service *Service) activeImageConfigFilename() string {
	runtime, err := service.runtimeForBackendMode(service.currentBackendMode(), readinessImage)
	if err != nil || runtime == nil {
		return ""
	}
	return currentRuntimeConfigFilename(runtime)
}

func (service *Service) activeTextConfigFilename() string {
	runtime, err := service.runtimeForBackendMode(service.currentBackendMode(), readinessText)
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
	if service.imageQueue == nil && service.textQueue == nil {
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
