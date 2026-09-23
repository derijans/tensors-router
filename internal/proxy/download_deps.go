package proxy

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/hardware"
)

type downloadDeps struct {
	service *Service
}

func (deps downloadDeps) SiteControlAllowed() bool {
	return deps.service.siteControlAllowed()
}

func (deps downloadDeps) RemoteInventoryURLs() []string {
	return deps.service.remoteInventoryURLs()
}

func (deps downloadDeps) NodeURLByID(nodeID string) string {
	return deps.service.nodeURLByID()[nodeID]
}

func (deps downloadDeps) ClusterClient() *cluster.Client {
	return deps.service.clusterClient
}

func (deps downloadDeps) ClusterRole() string {
	return deps.service.clusterRole
}

func (deps downloadDeps) NodeID() string {
	return deps.service.nodeID
}

func (deps downloadDeps) NodeURL() string {
	return deps.service.nodeURL
}

func (deps downloadDeps) Hardware() hardware.Source {
	return deps.service.hardware
}

func (deps downloadDeps) BackendMode() string {
	return deps.service.backendMode
}
