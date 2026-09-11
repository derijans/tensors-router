package cluster

import (
	"sort"

	"tensors-router/internal/schedulingcost"
)

// GroupMember names one model on one node within one lane. Membership is
// declared by an operator, so members are not required to share a name, a
// config hash, or a checkpoint. Lane keeps an image group and a text group
// from ever being confused even if their group id strings happened to
// collide, and lets one model that is both an LLM and an image model sit in a
// group per lane independently.
type GroupMember struct {
	Lane    string
	NodeID  string
	ModelID string
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
	PredictQueueMS(nodeID string, modelID string, lane string, count int64, work schedulingcost.Work) (float64, bool)
	PredictMS(nodeID string, modelID string, lane string, work schedulingcost.Work) (float64, bool)
	SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool)
	NodeBacklog(nodeID string, groupID string, lane string) (count int64, work schedulingcost.Work)
}

// RouteHint carries what selection needs to know about the request itself.
// Work is empty when the request body was never buffered, which turns cost
// ordering off for that request. RequiredContext is zero for the same reason,
// and for every lane that has no context window (image), which turns the
// context gate off with it: a request whose size was never measured is never
// gated on a guess. Whether the request may later be withdrawn for lending
// (false for streaming, since the router never buffered a replayable body for
// it) is not selection's concern and is decided by the queue instead.
type RouteHint struct {
	Work            schedulingcost.Work
	RequiredContext int
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

// groupMemberFor is the GroupMember a replica is addressed by in a given lane:
// its image id for the image lane, its own model id for every other lane.
func groupMemberFor(lane string, model Model) GroupMember {
	modelID := model.LocalID
	if lane == RouteLaneImage {
		modelID = model.ImageID
	}
	return GroupMember{Lane: lane, NodeID: model.NodeID, ModelID: modelID}
}

// groupExpandedReplicasLocked adds the members an operator declared
// interchangeable with the requested model, in the given lane. Without this a
// request can only ever reach replicas sharing its public id, which excludes
// the same checkpoint configured differently on another node.
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

// ContextFits reports whether a model's context window can hold a request that
// needs requiredContext tokens. A model that does not state its window
// (Context == 0) is unknown, never unlimited — exactly the model that would
// truncate silently if it were allowed through. requiredContext <= 0 means the
// request was never sized, which is not something ContextFits is ever asked to
// judge: callers gate on that before reaching here.
func ContextFits(model Model, requiredContext int) bool {
	if requiredContext <= 0 {
		return true
	}
	return model.Capabilities.Context > 0 && model.Capabilities.Context >= requiredContext
}

// selectGroupImageRouteLocked orders members by when each would finish this
// request: what it already has queued, plus a model load if it is holding
// something else, plus the request itself. It reports no choice unless every
// member is priced, so a group with an unmeasured member keeps the existing
// rotation and goes on building that member's history.
func (registry *Registry) selectGroupImageRouteLocked(groupID string, replicas []Model, localHealthy bool, hint RouteHint) (Route, bool) {
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
		count, work := registry.costs.NodeBacklog(replica.NodeID, groupID, RouteLaneImage)
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

// selectGroupTextRouteLocked mirrors selectGroupImageRouteLocked's cost
// ordering and adds the one thing the image lane never needed: a member whose
// context window cannot hold this request is dropped from contention before
// scoring, rather than scored and possibly chosen. If that leaves nobody, the
// caller falls back to plain rotation — the gate redirects work, it never
// refuses it; only bestOffloadHelper (a lease that costs nothing to refuse)
// is allowed to say no outright.
func (registry *Registry) selectGroupTextRouteLocked(groupID string, replicas []Model, localHealthy bool, hint RouteHint) (Route, bool) {
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
		count, work := registry.costs.NodeBacklog(replica.NodeID, groupID, RouteLaneText)
		backlogMS, ok := registry.costs.PredictQueueMS(replica.NodeID, replica.LocalID, RouteLaneText, count, work)
		if !ok {
			return Route{}, false
		}
		serviceMS, ok := registry.costs.PredictMS(replica.NodeID, replica.LocalID, RouteLaneText, hint.Work)
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
	return routeFromModel(selected, selected.NodeID != registry.localID, RouteLaneText), true
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

// withRequestedTextID is withRequestedImageID's text-lane twin: a member
// reached through a text group carries its own public id, and the response
// must still be attributed to the model the client actually asked for.
func withRequestedTextID(route Route, publicID string) Route {
	if publicID != "" {
		route.PublicID = publicID
	}
	return route
}
