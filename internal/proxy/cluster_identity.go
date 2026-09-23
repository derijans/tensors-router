package proxy

import "tensors-router/internal/cluster"

type clusterIdentity struct {
	nodeID   string
	client   *cluster.Client
	registry *cluster.Registry
}

func (service *Service) clusterIdentity() clusterIdentity {
	return clusterIdentity{nodeID: service.nodeID, client: service.clusterClient, registry: service.registry}
}
