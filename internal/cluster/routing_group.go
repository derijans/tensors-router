package cluster

import "sort"

// GroupMember names one image model on one node. Membership is declared by an
// operator, so members are not required to share a name, a config hash, or a
// checkpoint.
type GroupMember struct {
	NodeID  string
	ImageID string
}

// RouteGroupSource answers which models an operator has declared interchangeable.
// The registry consults it rather than reading a store, so selection stays free of
// SQL and stays testable on plain data.
type RouteGroupSource interface {
	GroupMembers(member GroupMember) (groupID string, members []GroupMember, ok bool)
}

// RouteCostSource prices a candidate. Every method reports whether it knows, and
// an unqualified candidate is never scheduled on a guess.
type RouteCostSource interface {
	PredictQueueMS(nodeID string, modelID string, lane string, count int64, work float64) (float64, bool)
	PredictMS(nodeID string, modelID string, lane string, work float64) (float64, bool)
	SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool)
	NodeBacklog(nodeID string, groupID string) (count int64, work float64)
}

// RouteHint carries what selection needs to know about the request itself. Work
// is the pre-dispatch size of an image job, and is zero when the request body was
// never buffered, which turns cost ordering off for that request.
type RouteHint struct {
	Work float64
}

func (registry *Registry) SetGroupSource(source RouteGroupSource) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.groups = source
}

// GroupMembers reports the group a model belongs to, for callers that need to
// know whether a request is subject to group scheduling before they route it.
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

// groupExpandedImageReplicasLocked adds the members an operator declared
// interchangeable with the requested model. Without this a request can only ever
// reach replicas sharing its public ID, which excludes the same checkpoint
// configured differently on another node.
func (registry *Registry) groupExpandedImageReplicasLocked(publicImageID string, replicas []Model, activeConfigFilename string) (string, []Model) {
	if registry.groups == nil || len(replicas) == 0 {
		return "", replicas
	}
	seen := map[GroupMember]struct{}{}
	for _, replica := range replicas {
		seen[GroupMember{NodeID: replica.NodeID, ImageID: replica.ImageID}] = struct{}{}
	}

	groupID := ""
	var members []GroupMember
	for _, replica := range replicas {
		id, groupMembers, ok := registry.groups.GroupMembers(GroupMember{NodeID: replica.NodeID, ImageID: replica.ImageID})
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
		if model.Disabled || model.NodeID != member.NodeID || model.ImageID != member.ImageID {
			continue
		}
		if !registry.imageModelSelectableLocked(model, activeConfigFilename) {
			continue
		}
		return model, true
	}
	return Model{}, false
}

// selectGroupImageRouteLocked orders members by when each would finish this
// request: what it already has queued, plus a model load if it is holding
// something else, plus the request itself. It reports no choice unless every
// member is priced, so a group with an unmeasured member keeps the existing
// rotation and goes on building that member's history.
func (registry *Registry) selectGroupImageRouteLocked(groupID string, replicas []Model, localHealthy bool, hint RouteHint) (Route, bool) {
	if registry.costs == nil || groupID == "" || hint.Work <= 0 || len(replicas) < 2 {
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
		count, work := registry.costs.NodeBacklog(replica.NodeID, groupID)
		backlogMS, ok := registry.costs.PredictQueueMS(replica.NodeID, replica.ImageID, RouteLaneImage, count, work)
		if !ok {
			return Route{}, false
		}
		serviceMS, ok := registry.costs.PredictMS(replica.NodeID, replica.ImageID, RouteLaneImage, hint.Work)
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
	return routeFromModel(selected, selected.NodeID != registry.localID, RouteLaneImage), true
}

// withRequestedImageID keeps the client's model identity on the route. A member
// reached through a group carries its own public image ID, and rewriting responses
// back to that one would answer a request for sdxl with a body naming sdxl-2.
func withRequestedImageID(route Route, publicImageID string) Route {
	if publicImageID != "" {
		route.PublicImageID = publicImageID
	}
	return route
}
