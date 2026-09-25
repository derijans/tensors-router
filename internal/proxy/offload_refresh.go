package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/schedulingcost"
)

var lendingLanes = []string{cluster.RouteLaneImage, cluster.RouteLaneText}

func (scheduler *scheduler) refreshOffloadPlan(ctx context.Context) {
	identity := scheduler.deps.clusterIdentity()
	if identity.role != cluster.RoleMaster || identity.registry == nil {
		return
	}
	snapshot, loaded := scheduler.deps.installStoredRoutingLinks(ctx)
	if !loaded {
		return
	}
	scheduler.deps.publishRoutingLinks(ctx, snapshot)
	plan := scheduler.planNow(ctx, identity, planTriggerTick)
	scheduler.deliverOffloadLeases(ctx, identity, plan.leases, queueEvent{})
}

func (scheduler *scheduler) planNow(ctx context.Context, identity clusterIdentity, trigger string) offloadPlan {
	scheduler.planMu.Lock()
	defer scheduler.planMu.Unlock()
	statuses := scheduler.collectRuntimeStatuses(ctx, identity.nodeID)
	scheduler.applyClusterCosts(statuses)

	costs := scheduler.costSource.Table()
	now := time.Now()
	index := scheduler.deps.routingLinkIndex()
	models := identity.registry.Models()
	policy := offloadPlanPolicy{ttl: scheduler.grantTTL, probeIdle: scheduler.probeIdle, trigger: trigger}
	var plan offloadPlan
	for _, lane := range lendingLanes {
		owners := lendingOwnersForLane(lane, models, index, statuses)
		lanePlan := planOffloadLeases(lane, owners, costs, now, policy)
		plan.leases = append(plan.leases, lanePlan.leases...)
		plan.decisions = append(plan.decisions, lanePlan.decisions...)
		plan.probeDueIn(lanePlan.nextProbeDue)
	}
	scheduler.leaseBook.Replace(plan.leases)
	for _, decision := range plan.decisions {
		scheduler.record(decision)
	}
	scheduler.scheduleProbeReplan(plan.nextProbeDue)
	return plan
}

func (scheduler *scheduler) scheduleProbeReplan(due time.Duration) {
	if scheduler.probeReplan != nil {
		scheduler.probeReplan.Stop()
		scheduler.probeReplan = nil
	}
	if due <= 0 || scheduler.lifetime.Err() != nil {
		return
	}
	scheduler.probeReplan = time.AfterFunc(due, scheduler.replanWhenProbeIsDue)
}

func (scheduler *scheduler) replanWhenProbeIsDue() {
	identity := scheduler.deps.clusterIdentity()
	if identity.role != cluster.RoleMaster || identity.registry == nil || scheduler.lifetime.Err() != nil {
		return
	}
	plan := scheduler.planNow(scheduler.lifetime, identity, planTriggerProbeDue)
	scheduler.deliverOffloadLeases(scheduler.lifetime, identity, plan.leases, queueEvent{Trigger: planTriggerProbeDue})
}

func (scheduler *scheduler) collectRuntimeStatuses(ctx context.Context, localNodeID string) map[string]NodeRuntimeStatus {
	statuses := scheduler.deps.remoteRuntimeStatuses(ctx)
	if statuses == nil {
		statuses = map[string]NodeRuntimeStatus{}
	}
	statuses[localNodeID] = scheduler.deps.localRuntimeStatus()
	return statuses
}

func (scheduler *scheduler) applyClusterCosts(statuses map[string]NodeRuntimeStatus) {
	costsByNode := make(map[string]schedulingcost.NodeCosts, len(statuses))
	for nodeID, status := range statuses {
		costsByNode[nodeID] = status.Costs
	}
	scheduler.costSource.Replace(schedulingcost.Merge(costsByNode))
}

func laneQueueStatsFor(status NodeRuntimeStatus, lane string, modelID string) offloadModelStats {
	queue := status.ImageQueue
	if lane == cluster.RouteLaneText {
		queue = status.TextQueue
	}
	for _, item := range queue {
		if item.ModelID == modelID {
			return item
		}
	}
	return offloadModelStats{}
}

func lendingOwnersForLane(lane string, registryModels []cluster.Model, index *routingLinkIndex, statuses map[string]NodeRuntimeStatus) []lendingOwner {
	models := lendableModelsByEndpoint(lane, registryModels)
	var owners []lendingOwner
	for _, ownerEndpoint := range index.owners(lane) {
		owner, ok := offloadCandidateFor(lane, ownerEndpoint, models, statuses)
		if !ok {
			continue
		}
		candidate := lendingOwner{owner: owner}
		for _, link := range index.lendTargets(lane, ownerEndpoint) {
			helper, ok := offloadCandidateFor(lane, link.Helper, models, statuses)
			if !ok {
				continue
			}
			candidate.helpers = append(candidate.helpers, offloadHelperCandidate{
				offloadCandidate:   helper,
				LoadIfUnloaded:     link.LoadIfUnloaded,
				RestoreAfterBorrow: link.RestoreAfterBorrow,
			})
		}
		if len(candidate.helpers) > 0 {
			owners = append(owners, candidate)
		}
	}
	return owners
}

func lendableModelsByEndpoint(lane string, models []cluster.Model) map[routinggroups.Endpoint]cluster.Model {
	byEndpoint := make(map[routinggroups.Endpoint]cluster.Model, len(models))
	for _, model := range models {
		if model.Disabled || !model.Available {
			continue
		}
		if lane == cluster.RouteLaneImage {
			if model.HasImage && model.ImageID != "" {
				byEndpoint[routinggroups.Endpoint{NodeID: model.NodeID, ModelID: model.ImageID}] = model
			}
			continue
		}
		if model.HasLLM && cluster.TextGroupEligible(model) {
			byEndpoint[routinggroups.Endpoint{NodeID: model.NodeID, ModelID: model.LocalID}] = model
		}
	}
	return byEndpoint
}

func offloadCandidateFor(lane string, endpoint routinggroups.Endpoint, models map[routinggroups.Endpoint]cluster.Model, statuses map[string]NodeRuntimeStatus) (offloadCandidate, bool) {
	model, known := models[endpoint]
	if !known {
		return offloadCandidate{}, false
	}
	status, reachable := statuses[endpoint.NodeID]
	if !reachable {
		return offloadCandidate{}, false
	}
	stats := laneQueueStatsFor(status, lane, endpoint.ModelID)
	candidate := offloadCandidate{
		NodeID:         endpoint.NodeID,
		ModelID:        endpoint.ModelID,
		ConfigFilename: model.Filename,
		Section:        laneSection(lane),
		IdleFor:        time.Duration(status.IdleForMS) * time.Millisecond,
		PendingCount:   stats.PendingCount,
		PendingWork:    stats.PendingWork,
		BacklogCount:   stats.BacklogCount,
		BacklogWork:    stats.BacklogWork,
	}
	if lane == cluster.RouteLaneText {
		candidate.Loaded = status.ActiveTextConfig == model.Filename
		candidate.AcceptingBorrowed = status.AcceptingBorrowedText
		candidate.ContextCapacity = model.Capabilities.Context
		candidate.PendingContext = stats.PendingContext
	} else {
		candidate.Loaded = status.ActiveImageConfig == model.Filename
		candidate.AcceptingBorrowed = status.AcceptingBorrowedImage
	}
	return candidate, true
}

func (scheduler *scheduler) deliverOffloadLeases(ctx context.Context, identity clusterIdentity, leases []offloadLease, answeredInline queueEvent) {
	nodeURLs := identity.registry.NodeURLsByID()
	trigger := planTriggerTick
	if answeredInline.Trigger != "" {
		trigger = answeredInline.Trigger
	}
	for _, lease := range leases {
		if lease.Lane == answeredInline.Lane && lease.OwnerNodeID == answeredInline.OwnerNodeID && lease.OwnerModelID == answeredInline.ModelID {
			continue
		}
		if lease.OwnerNodeID == identity.nodeID {
			scheduler.acceptOffloadLease(lease, trigger)
			continue
		}
		nodeURL := nodeURLs[lease.OwnerNodeID]
		if nodeURL == "" {
			continue
		}
		if err := identity.client.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/offload/grant", lease, nil); err != nil {
			scheduler.logger.Printf("offload grant delivery failed node=%s error=%v", lease.OwnerNodeID, err)
		}
	}
}

func (scheduler *scheduler) activeOffloadLease(lane string, modelID string, now time.Time) (offloadLease, bool) {
	value, ok := scheduler.offloadLeases.Load(laneModelKey(lane, modelID))
	if !ok {
		return offloadLease{}, false
	}
	lease, ok := value.(offloadLease)
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
