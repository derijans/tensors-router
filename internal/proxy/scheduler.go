package proxy

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/offloadsettings"
)

type schedulerDeps interface {
	clusterIdentity() clusterIdentity
	routingLinkIndex() *routingLinkIndex
	installStoredRoutingLinks(ctx context.Context) (routingLinkSnapshot, bool)
	publishRoutingLinks(ctx context.Context, snapshot routingLinkSnapshot)
	publishLendingSettings(ctx context.Context)
	remoteRuntimeStatuses(ctx context.Context) map[string]NodeRuntimeStatus
	localRuntimeStatus() NodeRuntimeStatus
	requestsRunningOnEveryBackendFamily() int
	ownWorkIdleSince() (time.Time, bool)
	acquireModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (*backendRuntime, func(), bool, error)
}

type decisionRecorder interface {
	Record(record offloaddecisions.Record)
}

type scheduler struct {
	settings      atomic.Pointer[offloadsettings.Settings]
	deps          schedulerDeps
	analytics     *requestAnalytics
	decisions     decisionRecorder
	logger        *log.Logger
	startedAt     time.Time
	imageQueue    *offloadQueue
	textQueue     *offloadQueue
	costSource    *schedulingCostSource
	leaseBook     *offloadLeaseBook
	offloadLeases sync.Map
	lent          *lentRequestBook
	dispatchMu    sync.Mutex
	planMu        sync.Mutex
	probeReplan   *time.Timer
	lifetime      context.Context
	endLifetime   context.CancelFunc
	eventReports  *queueEventCoalescer
	localCosts    atomic.Value
	borrowRestore sync.Map
	refreshCancel context.CancelFunc
	refreshDone   chan struct{}
	intervalReset chan struct{}
}

func newScheduler(deps schedulerDeps, analytics *requestAnalytics, decisions decisionRecorder, settings offloadsettings.Settings, logger *log.Logger) *scheduler {
	lifetime, endLifetime := context.WithCancel(context.Background())
	scheduler := &scheduler{
		lifetime:      lifetime,
		endLifetime:   endLifetime,
		deps:          deps,
		analytics:     analytics,
		decisions:     decisions,
		logger:        logger,
		startedAt:     time.Now(),
		imageQueue:    newOffloadQueue(settings.BackendDepth),
		textQueue:     newOffloadQueue(settings.BackendDepth),
		costSource:    newSchedulingCostSource(),
		leaseBook:     newOffloadLeaseBook(),
		lent:          newLentRequestBook(),
		intervalReset: make(chan struct{}, 1),
	}
	scheduler.settings.Store(&settings)
	scheduler.eventReports = newQueueEventCoalescer(scheduler.decideOnQueueEvent)
	scheduler.installAdmissionHolds()
	return scheduler
}

func (scheduler *scheduler) currentSettings() offloadsettings.Settings {
	return *scheduler.settings.Load()
}

func (scheduler *scheduler) applySettings(settings offloadsettings.Settings) {
	previous := scheduler.settings.Swap(&settings)
	scheduler.imageQueue.SetDepth(settings.BackendDepth)
	scheduler.textQueue.SetDepth(settings.BackendDepth)
	if previous.RefreshInterval != settings.RefreshInterval {
		select {
		case scheduler.intervalReset <- struct{}{}:
		default:
		}
	}
}

func (scheduler *scheduler) record(record offloaddecisions.Record) {
	if scheduler.decisions != nil {
		scheduler.decisions.Record(record)
	}
}

func (scheduler *scheduler) queueForLane(lane string) *offloadQueue {
	if lane == cluster.RouteLaneText {
		return scheduler.textQueue
	}
	return scheduler.imageQueue
}

func (scheduler *scheduler) close(ctx context.Context) error {
	err := scheduler.stopRefresh(ctx)
	scheduler.endLifetime()
	scheduler.stopProbeReplan()
	scheduler.eventReports.Close()
	scheduler.stopBorrowRestores()
	return err
}

func (scheduler *scheduler) stopProbeReplan() {
	scheduler.planMu.Lock()
	defer scheduler.planMu.Unlock()
	if scheduler.probeReplan != nil {
		scheduler.probeReplan.Stop()
	}
}
