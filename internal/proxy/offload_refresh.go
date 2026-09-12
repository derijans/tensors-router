package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/schedulingcost"
)

func (service *Service) refreshOffloadPlan(ctx context.Context) {
	if service.clusterRole != cluster.RoleMaster || service.registry == nil {
		return
	}
	statuses := service.collectRuntimeStatuses(ctx)
	service.applyClusterCosts(statuses)

	if service.routingGroups == nil {
		return
	}
	costs := service.costSource.Table()
	now := time.Now()
	var planned []offloadLease

	imageGroups, err := service.routingGroups.Groups(ctx)
	if err != nil {
		service.logger.Printf("offload plan skipped, routing groups unavailable: %v", err)
		return
	}
	for _, group := range imageGroups {
		members := make([]laneGroupMember, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, laneGroupMember{NodeID: member.NodeID, ModelID: member.ImageID})
		}
		candidates := service.offloadCandidatesForLane(cluster.RouteLaneImage, group.ID, members, statuses)
		planned = append(planned, planOffloadLeases(cluster.RouteLaneImage, group.ID, candidates, costs, now, service.schedulingGrantTTL)...)
	}

	textGroups, err := service.routingGroups.TextGroups(ctx)
	if err != nil {
		service.logger.Printf("offload plan skipped, text routing groups unavailable: %v", err)
		textGroups = nil
	}
	for _, group := range textGroups {
		members := make([]laneGroupMember, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, laneGroupMember{NodeID: member.NodeID, ModelID: member.ModelID})
		}
		candidates := service.offloadCandidatesForLane(cluster.RouteLaneText, group.ID, members, statuses)
		planned = append(planned, planOffloadLeases(cluster.RouteLaneText, group.ID, candidates, costs, now, service.schedulingGrantTTL)...)
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
	backlogs := make(map[string]map[string]offloadGroupStats, len(statuses))
	for nodeID, status := range statuses {
		costsByNode[nodeID] = status.Costs
		byGroup := make(map[string]offloadGroupStats, len(status.ImageQueue)+len(status.TextQueue))
		for _, stats := range status.ImageQueue {
			byGroup[backlogKey(cluster.RouteLaneImage, stats.GroupID)] = stats
		}
		for _, stats := range status.TextQueue {
			byGroup[backlogKey(cluster.RouteLaneText, stats.GroupID)] = stats
		}
		backlogs[nodeID] = byGroup
	}
	service.costSource.Replace(schedulingcost.Merge(costsByNode), backlogs)
}

type laneGroupMember struct {
	NodeID  string
	ModelID string
}

func laneGroupStatsFor(status NodeRuntimeStatus, lane string, groupID string) offloadGroupStats {
	queue := status.ImageQueue
	if lane == cluster.RouteLaneText {
		queue = status.TextQueue
	}
	for _, item := range queue {
		if item.GroupID == groupID {
			return item
		}
	}
	return offloadGroupStats{}
}

func (service *Service) offloadCandidatesForLane(lane string, groupID string, members []laneGroupMember, statuses map[string]NodeRuntimeStatus) []offloadCandidate {
	models := service.registry.Models()
	byMember := make(map[cluster.GroupMember]cluster.Model, len(models))
	for _, model := range models {
		if model.Disabled {
			continue
		}
		if lane == cluster.RouteLaneImage {
			if !model.HasImage || model.ImageID == "" {
				continue
			}
		} else if !model.HasLLM {
			continue
		}
		byMember[cluster.GroupMember{Lane: lane, NodeID: model.NodeID, ModelID: model.LocalID}] = model
		if lane == cluster.RouteLaneImage {
			byMember[cluster.GroupMember{Lane: lane, NodeID: model.NodeID, ModelID: model.ImageID}] = model
		}
	}

	candidates := make([]offloadCandidate, 0, len(members))
	for _, member := range members {
		model, known := byMember[cluster.GroupMember{Lane: lane, NodeID: member.NodeID, ModelID: member.ModelID}]
		if !known || !model.Available {
			continue
		}
		status, reachable := statuses[member.NodeID]
		if !reachable {
			continue
		}
		stats := laneGroupStatsFor(status, lane, groupID)
		candidate := offloadCandidate{
			NodeID:         member.NodeID,
			ModelID:        member.ModelID,
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
		candidates = append(candidates, candidate)
	}
	return candidates
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
	service.offloadLeases.Store(backlogKey(lease.Lane, lease.GroupID), lease)
}

func (service *Service) activeOffloadLease(lane string, groupID string, now time.Time) (offloadLease, bool) {
	value, ok := service.offloadLeases.Load(backlogKey(lane, groupID))
	if !ok {
		return offloadLease{}, false
	}
	lease, ok := value.(offloadLease)
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
