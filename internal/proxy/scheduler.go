package proxy

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
)

type schedulerDeps interface {
	clusterIdentity() clusterIdentity
	routingLinkIndex() *routingLinkIndex
	installStoredRoutingLinks(ctx context.Context) (routingLinkSnapshot, bool)
	publishRoutingLinks(ctx context.Context, snapshot routingLinkSnapshot)
	remoteRuntimeStatuses(ctx context.Context) map[string]NodeRuntimeStatus
	localRuntimeStatus() NodeRuntimeStatus
	requestsRunningOnEveryBackendFamily() int
	backendsIdleSince() (time.Time, bool)
	acquireModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (*backendRuntime, func(), bool, error)
}

type schedulingSettings struct {
	sampleWindow    time.Duration
	minSamples      int
	backendDepth    int
	refreshInterval time.Duration
	grantTTL        time.Duration
	contextReserve  int
	restoreDelay    time.Duration
	probeIdle       time.Duration
}

// withDefaults keeps a Service constructed without scheduling settings working,
// which is what every existing test does.
func (settings schedulingSettings) withDefaults() schedulingSettings {
	if settings.sampleWindow <= 0 {
		settings.sampleWindow = 24 * time.Hour
	}
	if settings.minSamples < 2 {
		settings.minSamples = 20
	}
	if settings.backendDepth < 1 {
		settings.backendDepth = 2
	}
	if settings.refreshInterval <= 0 {
		settings.refreshInterval = time.Minute
	}
	if settings.grantTTL <= 0 {
		settings.grantTTL = 30 * time.Second
	}
	if settings.contextReserve <= 0 {
		settings.contextReserve = 256
	}
	if settings.restoreDelay <= 0 {
		settings.restoreDelay = defaultOffloadRestoreDelay
	}
	if settings.probeIdle <= 0 {
		settings.probeIdle = defaultOffloadProbeIdle
	}
	return settings
}

type decisionRecorder interface {
	Record(record offloaddecisions.Record)
}

type scheduler struct {
	schedulingSettings
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
}

func newScheduler(deps schedulerDeps, analytics *requestAnalytics, decisions decisionRecorder, settings schedulingSettings, logger *log.Logger) *scheduler {
	settings = settings.withDefaults()
	lifetime, endLifetime := context.WithCancel(context.Background())
	scheduler := &scheduler{
		lifetime:           lifetime,
		endLifetime:        endLifetime,
		schedulingSettings: settings,
		deps:               deps,
		analytics:          analytics,
		decisions:          decisions,
		logger:             logger,
		startedAt:          time.Now(),
		imageQueue:         newOffloadQueue(settings.backendDepth),
		textQueue:          newOffloadQueue(settings.backendDepth),
		costSource:         newSchedulingCostSource(),
		leaseBook:          newOffloadLeaseBook(),
		lent:               newLentRequestBook(),
	}
	scheduler.eventReports = newQueueEventCoalescer(scheduler.decideOnQueueEvent)
	return scheduler
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
