package proxy

import "tensors-router/internal/cluster"

func (service *Service) nodeIDForURL(nodeURL string) string {
	if service.registry == nil {
		return ""
	}
	for nodeID, registeredURL := range service.registry.NodeURLsByID() {
		if cluster.BaseURLEqual(registeredURL, nodeURL) {
			return nodeID
		}
	}
	return ""
}
