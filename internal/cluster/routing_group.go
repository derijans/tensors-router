package cluster

import (
	"sort"

	"tensors-router/internal/schedulingcost"
)

type GroupMember struct {
	Lane    string
	NodeID  string
	ModelID string
}

type RouteGroupSource interface {
	GroupMembers(member GroupMember) (groupID string, members []GroupMember, ok bool)
}

type RouteCostSource interface {
	PredictQueueMS(nodeID string, modelID string, lane string, count int64, work schedulingcost.Work) (float64, bool)
	PredictMS(nodeID string, modelID string, lane string, work schedulingcost.Work) (float64, bool)
	SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool)
	NodeBacklog(nodeID string, groupID string, lane string) (count int64, work schedulingcost.Work)
}

type RouteHint struct {
	Work            schedulingcost.Work
	RequiredContext int
}

func UnsizedRouteHint() RouteHint {
	return RouteHint{}
}

func (registry *Registry) SetGroupSource(source RouteGroupSource) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.groups = source
}

func (registry *Registry) GroupMembers(member GroupMember) (string, []GroupMember, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.groups == nil {
		return "", nil, false
	}
	return registry.groups.GroupMembers(member)
}

func (registry *Registry) SetCostSource(source RouteCostSource) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.costs = source
}

func groupMemberFor(lane string, model Model) GroupMember {
	modelID := model.LocalID
	if lane == RouteLaneImage {
		modelID = model.ImageID
	}
	return GroupMember{Lane: lane, NodeID: model.NodeID, ModelID: modelID}
}

func (registry *Registry) groupExpandedReplicasLocked(lane string, publicID string, replicas []Model, activeConfigFilename string) (string, []Model) {
	if registry.groups == nil || len(replicas) == 0 {
		return "", replicas
	}
	seen := map[GroupMember]struct{}{}
	for _, replica := range replicas {
		seen[groupMemberFor(lane, replica)] = struct{}{}
	}

	groupID := ""
	var members []GroupMember
	for _, replica := range replicas {
		id, groupMembers, ok := registry.groups.GroupMembers(groupMemberFor(lane, replica))
		if ok {
			groupID, members = id, groupMembers
			break
		}
	}
	if groupID == "" {
		return "", replicas
	}

	expanded := append([]Model{}, replicas...)
	for _, member := range members {
		if _, already := seen[member]; already {
			continue
		}
		model, found := registry.modelForMemberLocked(member, activeConfigFilename)
		if !found {
			continue
		}
		seen[member] = struct{}{}
		expanded = append(expanded, model)
	}
	sort.Slice(expanded, func(left, right int) bool {
		return routeSortKey(expanded[left]) < routeSortKey(expanded[right])
	})
	return groupID, expanded
}

func (registry *Registry) modelForMemberLocked(member GroupMember, activeConfigFilename string) (Model, bool) {
	for _, model := range registry.view {
		if model.Disabled || model.NodeID != member.NodeID {
			continue
		}
		if member.Lane == RouteLaneImage {
			if model.ImageID != member.ModelID || !registry.imageModelSelectableLocked(model, activeConfigFilename) {
				continue
			}
		} else {
			if model.LocalID != member.ModelID || !model.Available || !TextGroupEligible(model) {
				continue
			}
		}
		return model, true
	}
	return Model{}, false
}

func ContextFits(model Model, requiredContext int) bool {
	if requiredContext <= 0 {
		return true
	}
	return model.Capabilities.Context > 0 && model.Capabilities.Context >= requiredContext
}

func (registry *Registry) selectGroupRouteLocked(lane string, groupID string, replicas []Model, localHealthy bool, hint RouteHint) (Route, bool) {
	if registry.costs == nil || groupID == "" || hint.Work.Arity() == 0 || len(replicas) < 2 {
		return Route{}, false
	}

	type scored struct {
		model Model
		score float64
	}
	candidates := make([]scored, 0, len(replicas))
	for _, replica := range replicas {
		if !replica.Available {
			return Route{}, false
		}
		if replica.NodeID == registry.localID && !localHealthy {
			return Route{}, false
		}
		modelID := groupMemberFor(lane, replica).ModelID
		count, work := registry.costs.NodeBacklog(replica.NodeID, groupID, lane)
		backlogMS, ok := registry.costs.PredictQueueMS(replica.NodeID, modelID, lane, count, work)
		if !ok {
			return Route{}, false
		}
		serviceMS, ok := registry.costs.PredictMS(replica.NodeID, modelID, lane, hint.Work)
		if !ok {
			return Route{}, false
		}
		switchMS := 0.0
		if !replica.Loaded {
			switchMS, ok = registry.costs.SwitchPenaltyMS(replica.NodeID, replica.Filename)
			if !ok {
				return Route{}, false
			}
		}
		if !ContextFits(replica, hint.RequiredContext) {
			continue
		}
		candidates = append(candidates, scored{model: replica, score: backlogMS + switchMS + serviceMS})
	}
	if len(candidates) == 0 {
		return Route{}, false
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].score != candidates[right].score {
			return candidates[left].score < candidates[right].score
		}
		return routeSortKey(candidates[left].model) < routeSortKey(candidates[right].model)
	})
	selected := candidates[0].model
	return routeFromModel(selected, selected.NodeID != registry.localID, lane), true
}

func withRequestedPublicID(route Route, lane string, publicID string) Route {
	if publicID == "" {
		return route
	}
	if lane == RouteLaneImage {
		route.PublicImageID = publicID
	} else {
		route.PublicID = publicID
	}
	return route
}
