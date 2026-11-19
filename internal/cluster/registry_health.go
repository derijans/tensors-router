package cluster

func (registry *Registry) PeerHealth() map[string]bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	health := make(map[string]bool, len(registry.nodes))
	for nodeID := range registry.nodes {
		_, unhealthy := registry.unhealthy[nodeID]
		health[nodeID] = !unhealthy
	}
	return health
}
