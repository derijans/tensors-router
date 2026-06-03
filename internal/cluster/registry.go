package cluster

import (
	"fmt"
	"sort"
	"sync"

	"tensors-router/internal/catalog"
)

type Registry struct {
	mu        sync.Mutex
	role      string
	localID   string
	localURL  string
	local     []Model
	nodes     map[string]Snapshot
	unhealthy map[string]struct{}
	busy      map[string]int
	next      map[string]int
	view      []Model
	store     *Store
}

func NewRegistry(role string, localID string, localURL string, store *Store) *Registry {
	return &Registry{
		role:      role,
		localID:   localID,
		localURL:  localURL,
		nodes:     map[string]Snapshot{},
		unhealthy: map[string]struct{}{},
		busy:      map[string]int{},
		next:      map[string]int{},
		store:     store,
	}
}

func (registry *Registry) UpdateLocal(models []Model) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.local = cloneModels(models)
	registry.rebuildLocked()
	return registry.saveLocked()
}

func (registry *Registry) UpdateNode(snapshot Snapshot) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if snapshot.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	registry.nodes[snapshot.NodeID] = normalizeNodeSnapshot(snapshot)
	delete(registry.unhealthy, snapshot.NodeID)
	registry.rebuildLocked()
	return registry.saveLocked()
}

func (registry *Registry) MarkNodeHealth(nodeID string, healthy bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if healthy {
		delete(registry.unhealthy, nodeID)
	} else {
		registry.unhealthy[nodeID] = struct{}{}
	}
	registry.rebuildLocked()
	_ = registry.saveLocked()
}

func (registry *Registry) MarkNodeURLHealth(nodeURL string, healthy bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for nodeID, snapshot := range registry.nodes {
		if snapshot.NodeURL != nodeURL {
			continue
		}
		if healthy {
			delete(registry.unhealthy, nodeID)
		} else {
			registry.unhealthy[nodeID] = struct{}{}
		}
	}
	registry.rebuildLocked()
	_ = registry.saveLocked()
}

func (registry *Registry) Snapshot() Snapshot {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	return Snapshot{
		NodeID:  registry.localID,
		NodeURL: registry.localURL,
		Models:  cloneModels(registry.local),
	}
}

func (registry *Registry) Models() []Model {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	return cloneModels(registry.view)
}

func (registry *Registry) HasModel(publicID string) bool {
	_, ok := registry.Model(publicID)
	return ok
}

func (registry *Registry) HasImageModel(publicImageID string, activeConfigFilename string) bool {
	_, ok := registry.ImageModel(publicImageID, activeConfigFilename)
	return ok
}

func (registry *Registry) Model(publicID string) (Model, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for _, model := range registry.view {
		if model.PublicID == publicID {
			return model, true
		}
	}
	return Model{}, false
}

func (registry *Registry) ImageModel(publicImageID string, activeConfigFilename string) (Model, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for _, model := range registry.view {
		if registry.imageModelSelectableLocked(model, activeConfigFilename) && model.PublicImageID == publicImageID {
			return model, true
		}
	}
	return Model{}, false
}

func (registry *Registry) Acquire(publicID string, localHealthy bool) (Route, func(), bool) {
	registry.mu.Lock()
	route, ok := registry.selectRouteLocked(publicID, registry.replicasLocked(publicID), localHealthy, RouteLaneText)
	if !ok {
		registry.mu.Unlock()
		return Route{}, func() {}, false
	}
	return registry.acquireRouteLocked(route)
}

func (registry *Registry) AcquireImage(publicImageID string, localHealthy bool, activeConfigFilename string) (Route, func(), bool) {
	registry.mu.Lock()
	route, ok := registry.selectRouteLocked(publicImageID, registry.imageReplicasLocked(publicImageID, activeConfigFilename), localHealthy, RouteLaneImage)
	if !ok {
		registry.mu.Unlock()
		return Route{}, func() {}, false
	}
	return registry.acquireRouteLocked(route)
}

func (registry *Registry) acquireRouteLocked(route Route) (Route, func(), bool) {
	key := routeKey(route)
	registry.busy[key]++
	registry.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			registry.mu.Lock()
			if registry.busy[key] > 0 {
				registry.busy[key]--
			}
			registry.mu.Unlock()
		})
	}
	return route, release, true
}

func (registry *Registry) rebuildLocked() {
	candidates := registry.candidatesLocked()
	assignPublicIDs(candidates)
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].PublicID == candidates[right].PublicID {
			return routeSortKey(candidates[left]) < routeSortKey(candidates[right])
		}
		return candidates[left].PublicID < candidates[right].PublicID
	})
	registry.view = candidates
}

func (registry *Registry) candidatesLocked() []Model {
	candidates := cloneModels(registry.local)
	for index := range candidates {
		candidates[index].Source = registry.localSource()
		candidates[index].NodeID = registry.localID
		candidates[index].NodeURL = registry.localURL
		candidates[index].Available = true
	}

	nodeIDs := make([]string, 0, len(registry.nodes))
	for nodeID := range registry.nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		snapshot := registry.nodes[nodeID]
		_, unhealthy := registry.unhealthy[nodeID]
		for _, model := range snapshot.Models {
			model.Source = SourceSlave
			model.NodeID = snapshot.NodeID
			model.NodeURL = snapshot.NodeURL
			model.Available = !unhealthy
			candidates = append(candidates, model)
		}
	}
	return candidates
}

func (registry *Registry) localSource() string {
	if registry.role == RoleMaster {
		return SourceMaster
	}
	return SourceLocal
}

func (registry *Registry) selectRouteLocked(publicID string, replicas []Model, localHealthy bool, lane string) (Route, bool) {
	if len(replicas) == 0 {
		return Route{}, false
	}
	for _, replica := range replicas {
		route := routeFromModel(replica, false, lane)
		if replica.NodeID == registry.localID && replica.Available && localHealthy && registry.busy[routeKey(route)] == 0 {
			return route, true
		}
	}

	slaves := make([]Model, 0, len(replicas))
	for _, replica := range replicas {
		if replica.NodeID != registry.localID && replica.Available {
			slaves = append(slaves, replica)
		}
	}
	if len(slaves) > 0 {
		index := registry.next[publicID] % len(slaves)
		registry.next[publicID] = (registry.next[publicID] + 1) % len(slaves)
		return routeFromModel(slaves[index], true, lane), true
	}

	for _, replica := range replicas {
		if replica.NodeID == registry.localID && replica.Available && localHealthy {
			return routeFromModel(replica, false, lane), true
		}
	}
	return Route{}, false
}

func (registry *Registry) replicasLocked(publicID string) []Model {
	replicas := make([]Model, 0)
	for _, model := range registry.view {
		if model.PublicID == publicID {
			replicas = append(replicas, model)
		}
	}
	sort.Slice(replicas, func(left, right int) bool {
		return routeSortKey(replicas[left]) < routeSortKey(replicas[right])
	})
	return replicas
}

func (registry *Registry) imageReplicasLocked(publicImageID string, activeConfigFilename string) []Model {
	replicas := make([]Model, 0)
	for _, model := range registry.view {
		if model.PublicImageID == publicImageID && registry.imageModelSelectableLocked(model, activeConfigFilename) {
			replicas = append(replicas, model)
		}
	}
	sort.Slice(replicas, func(left, right int) bool {
		return routeSortKey(replicas[left]) < routeSortKey(replicas[right])
	})
	return replicas
}

func (registry *Registry) imageModelSelectableLocked(model Model, activeConfigFilename string) bool {
	if !model.HasImage || model.PublicImageID == "" {
		return false
	}
	if !model.HasLLM {
		return true
	}
	if model.BackendMode == BackendModeLlamaSDCPP {
		return true
	}
	if activeConfigFilename == catalog.AllImageConfigs && model.BackendMode == BackendModeLlamaSDCPP {
		return true
	}
	if model.NodeID != registry.localID {
		return true
	}
	return model.Filename == activeConfigFilename
}

func (registry *Registry) saveLocked() error {
	if registry.store == nil {
		return nil
	}
	return registry.store.Save(Snapshot{
		NodeID:  registry.localID,
		NodeURL: registry.localURL,
		Models:  cloneModels(registry.view),
	})
}

func normalizeNodeSnapshot(snapshot Snapshot) Snapshot {
	normalized := Snapshot{
		NodeID:  snapshot.NodeID,
		NodeURL: snapshot.NodeURL,
		Models:  cloneModels(snapshot.Models),
	}
	for index := range normalized.Models {
		normalized.Models[index].NodeID = normalized.NodeID
		normalized.Models[index].NodeURL = normalized.NodeURL
		normalized.Models[index].PublicID = normalized.Models[index].LocalID
		normalized.Models[index].PublicImageID = normalized.Models[index].ImageID
		if normalized.Models[index].BackendMode == "" {
			normalized.Models[index].BackendMode = BackendModeKobold
		}
	}
	return normalized
}

func cloneModels(models []Model) []Model {
	cloned := make([]Model, len(models))
	copy(cloned, models)
	return cloned
}
