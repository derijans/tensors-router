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

func (service *Service) refreshOffloadPlan(ctx context.Context) {
	if service.clusterRole != cluster.RoleMaster || service.registry == nil {
		return
	}
	statuses := service.collectRuntimeStatuses(ctx)
	service.applyClusterCosts(statuses)

	snapshot, loaded := service.installStoredRoutingLinks(ctx)
	if !loaded {
		return
	}
	service.publishRoutingLinks(ctx, snapshot)

	costs := service.costSource.Table()
	now := time.Now()
	index := service.routingLinkIndex()
	var planned []offloadLease
	for _, lane := range lendingLanes {
		owners := service.lendingOwnersForLane(lane, index, statuses)
		planned = append(planned, planOffloadLeases(lane, owners, costs, now, service.schedulingGrantTTL)...)
	}
	service.leaseBook.Replace(planned)
	service.deliverOffloadLeases(ctx, planned)
}

func (service *Service) collectRuntimeStatuses(ctx context.Context) map[string]NodeRuntimeStatus {
	statuses := service.remoteRuntimeStatuses(ctx)
	if statuses == nil {
		statuses = map[string]NodeRuntimeStatus{}
	}
	local := service.localRuntimeStatus()
	statuses[service.nodeID] = local
	return statuses
}

func (service *Service) applyClusterCosts(statuses map[string]NodeRuntimeStatus) {
	costsByNode := make(map[string]schedulingcost.NodeCosts, len(statuses))
	for nodeID, status := range statuses {
		costsByNode[nodeID] = status.Costs
	}
	service.costSource.Replace(schedulingcost.Merge(costsByNode))
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

func (service *Service) lendingOwnersForLane(lane string, index *routingLinkIndex, statuses map[string]NodeRuntimeStatus) []lendingOwner {
	models := lendableModelsByEndpoint(lane, service.registry.Models())
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

func (service *Service) deliverOffloadLeases(ctx context.Context, leases []offloadLease) {
	nodeURLs := service.registry.NodeURLsByID()
	for _, lease := range leases {
		if lease.OwnerNodeID == service.nodeID {
			service.storeOffloadLease(lease)
			continue
		}
		nodeURL := nodeURLs[lease.OwnerNodeID]
		if nodeURL == "" {
			continue
		}
		if err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/offload/grant", lease, nil); err != nil {
			service.logger.Printf("offload grant delivery failed node=%s error=%v", lease.OwnerNodeID, err)
		}
	}
}

func (service *Service) storeOffloadLease(lease offloadLease) {
	service.offloadLeases.Store(laneModelKey(lease.Lane, lease.OwnerModelID), lease)
}

func (service *Service) activeOffloadLease(lane string, modelID string, now time.Time) (offloadLease, bool) {
	value, ok := service.offloadLeases.Load(laneModelKey(lane, modelID))
	if !ok {
		return offloadLease{}, false
	}
	lease, ok := value.(offloadLease)
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
