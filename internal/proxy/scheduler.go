package proxy

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"tensors-router/internal/cluster"
)

type schedulerDeps interface {
	clusterIdentity() clusterIdentity
	routingLinkIndex() *routingLinkIndex
	installStoredRoutingLinks(ctx context.Context) (routingLinkSnapshot, bool)
	publishRoutingLinks(ctx context.Context, snapshot routingLinkSnapshot)
	remoteRuntimeStatuses(ctx context.Context) map[string]NodeRuntimeStatus
	localRuntimeStatus() NodeRuntimeStatus
	requestsRunningOnEveryBackendFamily() int
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
	return settings
}

type scheduler struct {
	schedulingSettings
	deps            schedulerDeps
	analytics       *requestAnalytics
	logger          *log.Logger
	imageQueue      *offloadQueue
	textQueue       *offloadQueue
	costSource      *schedulingCostSource
	leaseBook       *offloadLeaseBook
	offloadLeases   sync.Map
	offloadInFlight sync.Map
	localCosts      atomic.Value
	borrowRestore   sync.Map
	refreshCancel   context.CancelFunc
	refreshDone     chan struct{}
}

func newScheduler(deps schedulerDeps, analytics *requestAnalytics, settings schedulingSettings, logger *log.Logger) *scheduler {
	settings = settings.withDefaults()
	return &scheduler{
		schedulingSettings: settings,
		deps:               deps,
		analytics:          analytics,
		logger:             logger,
		imageQueue:         newOffloadQueue(settings.backendDepth),
		textQueue:          newOffloadQueue(settings.backendDepth),
		costSource:         newSchedulingCostSource(),
		leaseBook:          newOffloadLeaseBook(),
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
	scheduler.stopBorrowRestores()
	return err
}
