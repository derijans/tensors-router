package proxy

import "tensors-router/internal/cluster"

type clusterIdentity struct {
	nodeID    string
	nodeURL   string
	masterURL string
	role      string
	client    *cluster.Client
	registry  *cluster.Registry
}

func (service *Service) clusterIdentity() clusterIdentity {
	return clusterIdentity{
		nodeID:    service.nodeID,
		nodeURL:   service.nodeURL,
		masterURL: service.masterURL,
		role:      service.clusterRole,
		client:    service.clusterClient,
		registry:  service.registry,
	}
}

func (service *Service) nodeProxyToken() string {
	return service.clusterToken
}
