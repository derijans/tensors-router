package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/schedulingcost"
)

// refreshOffloadPlan is the master's cycle: collect what every node is holding,
// rebuild the shared cost picture from the coefficients they published, then decide
// which owners may lend to which helpers and tell them so.
func (service *Service) refreshOffloadPlan(ctx context.Context) {
	if service.clusterRole != cluster.RoleMaster || service.registry == nil {
		return
	}
	statuses := service.collectRuntimeStatuses(ctx)
	service.applyClusterCosts(statuses)

	if service.routingGroups == nil {
		return
	}
	groups, err := service.routingGroups.Groups(ctx)
	if err != nil {
		service.logger.Printf("offload plan skipped, routing groups unavailable: %v", err)
		return
	}
	costs := service.costSource.Table()
	now := time.Now()
	var planned []offloadLease
	for _, group := range groups {
		candidates := service.offloadCandidates(group, statuses)
		planned = append(planned, planOffloadLeases(group.ID, candidates, costs, now, service.schedulingGrantTTL)...)
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

// applyClusterCosts merges every node's published coefficients into one table and
// records what each node has queued, so selection prices candidates from one
// coherent snapshot.
func (service *Service) applyClusterCosts(statuses map[string]NodeRuntimeStatus) {
	costsByNode := make(map[string]schedulingcost.NodeCosts, len(statuses))
	backlogs := make(map[string]map[string]offloadGroupStats, len(statuses))
	for nodeID, status := range statuses {
		costsByNode[nodeID] = status.Costs
		byGroup := make(map[string]offloadGroupStats, len(status.ImageQueue))
		for _, stats := range status.ImageQueue {
			byGroup[stats.GroupID] = stats
		}
		backlogs[nodeID] = byGroup
	}
	service.costSource.Replace(schedulingcost.Merge(costsByNode), backlogs)
}

func (service *Service) offloadCandidates(group routinggroups.Group, statuses map[string]NodeRuntimeStatus) []offloadCandidate {
	models := service.registry.Models()
	byMember := make(map[cluster.GroupMember]cluster.Model, len(models))
	for _, model := range models {
		if model.Disabled || !model.HasImage || model.ImageID == "" {
			continue
		}
		byMember[cluster.GroupMember{NodeID: model.NodeID, ImageID: model.ImageID}] = model
	}

	candidates := make([]offloadCandidate, 0, len(group.Members))
	for _, member := range group.Members {
		model, known := byMember[cluster.GroupMember{NodeID: member.NodeID, ImageID: member.ImageID}]
		if !known || !model.Available {
			continue
		}
		status, reachable := statuses[member.NodeID]
		if !reachable {
			continue
		}
		stats := offloadGroupStats{}
		for _, item := range status.ImageQueue {
			if item.GroupID == group.ID {
				stats = item
				break
			}
		}
		candidates = append(candidates, offloadCandidate{
			NodeID:            member.NodeID,
			ModelID:           member.ImageID,
			ConfigFilename:    model.Filename,
			Loaded:            status.ActiveImageConfig == model.Filename,
			AcceptingBorrowed: status.AcceptingBorrowed,
			PendingCount:      stats.PendingCount,
			PendingWork:       stats.PendingWork,
			BacklogCount:      stats.BacklogCount,
			BacklogWork:       stats.BacklogWork,
		})
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
	service.offloadLeases.Store(lease.GroupID, lease)
}

// activeOffloadLease reports the lease this node may currently use for a group. An
// expired lease is simply absent, so a helper that went busy or a master that
// stopped polling ends the arrangement without a revoke having to arrive.
func (service *Service) activeOffloadLease(groupID string, now time.Time) (offloadLease, bool) {
	value, ok := service.offloadLeases.Load(groupID)
	if !ok {
		return offloadLease{}, false
	}
	lease, ok := value.(offloadLease)
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
