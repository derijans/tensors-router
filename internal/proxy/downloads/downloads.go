package downloads

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/hardware"
)

type Deps interface {
	SiteControlAllowed() bool
	RemoteInventoryURLs() []string
	NodeURLByID(nodeID string) string
	ClusterClient() *cluster.Client
	ClusterRole() string
	NodeID() string
	NodeURL() string
	Hardware() hardware.Source
	BackendMode() string
}

type Handlers struct {
	deps       Deps
	downloader downloader.Service
	capability downloader.Capability
}

func New(deps Deps, service downloader.Service, capability downloader.Capability) *Handlers {
	if service != nil {
		capability = downloader.MergeRuntimeCapability(capability, service.Capability())
	}
	return &Handlers{deps: deps, downloader: service, capability: capability}
}

func (handlers *Handlers) Downloader() downloader.Service {
	return handlers.downloader
}
