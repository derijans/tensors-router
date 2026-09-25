package proxy

import (
	"context"
	"time"

	"tensors-router/internal/schedulingcost"
)

func (scheduler *scheduler) applyQueueStatus(status *NodeRuntimeStatus) {
	activity := scheduler.nodeActivity()
	status.ImageQueue = scheduler.imageQueue.Stats()
	status.AcceptingBorrowedImage = scheduler.imageQueue.AcceptingBorrowed(activity)
	status.TextQueue = scheduler.textQueue.Stats()
	status.AcceptingBorrowedText = scheduler.textQueue.AcceptingBorrowed(activity)
	status.IdleForMS = scheduler.idleFor(time.Now()).Milliseconds()
	if costs := scheduler.publishedCosts(); costs != nil {
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
func (scheduler *scheduler) publishedCosts() *schedulingcost.NodeCosts {
	value := scheduler.localCosts.Load()
	if value == nil {
		return nil
	}
	costs, ok := value.(*schedulingcost.NodeCosts)
	if !ok {
		return nil
	}
	return costs
}

func (scheduler *scheduler) refreshLocalCosts(ctx context.Context) {
	costs := scheduler.fitLocalCosts(ctx)
	scheduler.localCosts.Store(&costs)
}

// StartSchedulingRefresh keeps the fitted costs current and, on a master, keeps
// offload leases current with them. It returns immediately; the loop stops with
// the context or with Close, whichever comes first.
func (service *Service) StartSchedulingRefresh(ctx context.Context) {
	service.scheduler.startRefresh(ctx)
}

func (scheduler *scheduler) startRefresh(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	scheduler.refreshCancel = cancel
	scheduler.refreshDone = done
	go func() {
		defer close(done)
		scheduler.runRefresh(ctx)
	}()
}

func (scheduler *scheduler) runRefresh(ctx context.Context) {
	ticker := time.NewTicker(scheduler.refreshInterval)
	defer ticker.Stop()
	scheduler.refreshLocalCosts(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scheduler.refreshLocalCosts(ctx)
			scheduler.refreshOffloadPlan(ctx)
		}
	}
}

func (scheduler *scheduler) stopRefresh(ctx context.Context) error {
	if scheduler.refreshCancel == nil {
		return nil
	}
	scheduler.refreshCancel()
	select {
	case <-scheduler.refreshDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
