package proxy

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/proxy/routing"
)

var errAssetManagerClosed = errors.New("model asset manager is closed")

type assetDeps interface {
	configNodeTarget(nodeID string, nodeURL string) (configNodeTarget, error)
	localConfigFileTarget(filename string) (string, error)
	refreshLocalRegistry() error
	localClusterModels() ([]cluster.Model, error)
	siteControlAllowed() bool
	clusterIdentity() clusterIdentity
	maxTransferBytes() int64
}

type assetManager struct {
	deps            assetDeps
	index           *modelassets.Index
	fileRoots       []string
	downloader      downloader.Service
	configLocks     sync.Map
	resolutionJobs  sync.Map
	transfers       sync.Map
	transferSlots   chan struct{}
	lookupMu        sync.Mutex
	lookupCache     map[string]assetLookupCacheEntry
	lookupTimeout   time.Duration
	transferTimeout time.Duration
	jobsMu          sync.Mutex
	jobsClosed      bool
	jobs            sync.WaitGroup
	logger          *log.Logger
}

type assetManagerConfig struct {
	index              *modelassets.Index
	fileRoots          []string
	downloader         downloader.Service
	concurrentTransfer int
	logger             *log.Logger
}

func newAssetManager(deps assetDeps, config assetManagerConfig) *assetManager {
	return &assetManager{
		deps:            deps,
		index:           config.index,
		fileRoots:       append([]string{}, config.fileRoots...),
		downloader:      config.downloader,
		transferSlots:   make(chan struct{}, config.concurrentTransfer),
		lookupCache:     make(map[string]assetLookupCacheEntry),
		lookupTimeout:   assetLookupTimeout,
		transferTimeout: modelOperationTimeout,
		logger:          config.logger,
	}
}

func (assets *assetManager) identity() clusterIdentity {
	return assets.deps.clusterIdentity()
}

func (assets *assetManager) trackJob() bool {
	assets.jobsMu.Lock()
	defer assets.jobsMu.Unlock()
	if assets.jobsClosed {
		return false
	}
	assets.jobs.Add(1)
	return true
}

func (assets *assetManager) close(ctx context.Context) error {
	assets.jobsMu.Lock()
	assets.jobsClosed = true
	assets.jobsMu.Unlock()
	finished := make(chan struct{})
	go func() {
		assets.jobs.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (assets *assetManager) routes() []routing.Route {
	return append([]routing.Route{
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/export", assets.handleSiteModelAssetExport),
		routing.Exact(http.MethodPost, "/router/v1/site/model-files/hash", assets.handleSiteModelFileHash),
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/resolve", assets.handleSiteModelAssetResolve),
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/resolve-batch", assets.handleSiteModelAssetResolveBatch),
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/jobs", assets.handleSiteModelAssetCreateJob),
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/bind", assets.handleSiteModelAssetBinding),
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/candidates", assets.handleSiteModelAssetCandidates),
		routing.Exact(http.MethodPost, "/router/v1/site/model-assets/substitute", assets.handleSiteModelAssetSubstitution),
		routing.Prefix(http.MethodGet, "/router/v1/site/model-assets/jobs/", assets.handleSiteModelAssetJob),
		routing.Prefix(http.MethodGet, "/router/v1/site/model-assets/", assets.handleSiteModelAssetLookup),
	}, routing.ClusterOnly([]routing.Route{
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-assets/resolve", assets.handleNodeModelAssetResolve),
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-assets/export", assets.handleNodeModelAssetExport),
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-files/hash", assets.handleNodeModelFileHash),
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-assets/jobs", assets.handleNodeModelAssetCreateJob),
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-assets/bind", assets.handleNodeModelAssetBinding),
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-assets/candidates", assets.handleNodeModelAssetCandidates),
		routing.Exact(http.MethodPost, "/router/v1/node/site/model-assets/substitute", assets.handleNodeModelAssetSubstitution),
		routing.Prefix(http.MethodGet, "/router/v1/node/site/model-assets/jobs/", assets.handleNodeModelAssetJob),
		routing.Exact(http.MethodPost, "/router/v1/node/assets/lookup", assets.handleNodeAssetLookup),
		routing.Exact(http.MethodPost, "/router/v1/node/assets/lookup-cluster", assets.handleNodeClusterAssetLookup),
		routing.Prefix(http.MethodGet, "/router/v1/node/assets/", assets.handleNodeAssetStream),
	})...)
}
