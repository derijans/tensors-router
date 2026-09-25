package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/openai"
	"tensors-router/internal/routinggroups"
)

const offloadEventPath = "/router/v1/node/offload/event"

func (scheduler *scheduler) reportQueueEvent(lane string, modelID string, trigger string) {
	identity := scheduler.deps.clusterIdentity()
	if identity.role != cluster.RoleMaster && identity.role != cluster.RoleSlave {
		return
	}
	owner := routinggroups.Endpoint{NodeID: identity.nodeID, ModelID: modelID}
	if len(scheduler.deps.routingLinkIndex().lendTargets(lane, owner)) == 0 {
		return
	}
	if !scheduler.queueForLane(lane).HoldsPendingWork(modelID) && scheduler.lent.Count(lane, modelID) == 0 {
		return
	}
	scheduler.eventReports.Report(queueEvent{Lane: lane, OwnerNodeID: identity.nodeID, ModelID: modelID, Trigger: trigger})
}

func (scheduler *scheduler) decideOnQueueEvent(ctx context.Context, event queueEvent) {
	identity := scheduler.deps.clusterIdentity()
	switch identity.role {
	case cluster.RoleMaster:
		lease, granted := scheduler.planForEvent(ctx, identity, event)
		scheduler.applyDecidedLease(event, lease, granted)
	case cluster.RoleSlave:
		var decision queueEventDecision
		if err := identity.client.JSON(ctx, http.MethodPost, identity.masterURL, offloadEventPath, event, &decision); err != nil {
			if ctx.Err() == nil {
				scheduler.logger.Printf("offload event delivery failed lane=%s model=%q error=%v", event.Lane, event.ModelID, err)
			}
			return
		}
		scheduler.applyDecidedLease(event, derefLease(decision.Lease), decision.Lease != nil)
	}
}

func (scheduler *scheduler) planForEvent(ctx context.Context, identity clusterIdentity, event queueEvent) (offloadLease, bool) {
	if identity.registry == nil {
		return offloadLease{}, false
	}
	plan := scheduler.planNow(ctx, identity, event.Trigger)
	scheduler.deliverOffloadLeases(ctx, identity, plan.leases, event)
	for _, lease := range plan.leases {
		if lease.Lane == event.Lane && lease.OwnerNodeID == event.OwnerNodeID && lease.OwnerModelID == event.ModelID {
			return lease, true
		}
	}
	return offloadLease{}, false
}

func (scheduler *scheduler) applyDecidedLease(event queueEvent, lease offloadLease, granted bool) {
	if granted {
		scheduler.acceptOffloadLease(lease, event.Trigger)
		return
	}
	if _, held := scheduler.offloadLeases.LoadAndDelete(laneModelKey(event.Lane, event.ModelID)); held {
		scheduler.record(offloaddecisions.Record{
			Kind:         offloaddecisions.KindDispatch,
			Trigger:      event.Trigger,
			Lane:         event.Lane,
			OwnerModelID: event.ModelID,
			Outcome:      offloaddecisions.OutcomeLeaseCleared,
			LentOut:      scheduler.lent.Count(event.Lane, event.ModelID),
		})
	}
}

func (scheduler *scheduler) acceptOffloadLease(lease offloadLease, trigger string) {
	scheduler.offloadLeases.Store(laneModelKey(lease.Lane, lease.OwnerModelID), lease)
	scheduler.record(offloaddecisions.Record{
		Kind:          offloaddecisions.KindDispatch,
		Trigger:       trigger,
		Lane:          lease.Lane,
		OwnerNodeID:   lease.OwnerNodeID,
		OwnerModelID:  lease.OwnerModelID,
		HelperNodeID:  lease.HelperNodeID,
		HelperModelID: lease.HelperModelID,
		Outcome:       offloaddecisions.OutcomeLeaseUpdated,
		Slots:         lease.slots(),
		LentOut:       scheduler.lent.Count(lease.Lane, lease.OwnerModelID),
	})
	scheduler.maybeOffload(lease.Lane, lease.OwnerModelID)
}

func (service *Service) handleNodeOffloadEvent(w http.ResponseWriter, r *http.Request) {
	var event queueEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	event.OwnerNodeID = strings.TrimSpace(event.OwnerNodeID)
	event.ModelID = strings.TrimSpace(event.ModelID)
	if event.OwnerNodeID == "" || event.ModelID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "owner_node_id and model_id are required")
		return
	}
	event.Lane = offloadLaneFromHeader(event.Lane)
	identity := service.clusterIdentity()
	if identity.role != cluster.RoleMaster {
		openai.WriteError(w, http.StatusConflict, "not_master", "offload decisions are made by the master")
		return
	}
	lease, granted := service.scheduler.planForEvent(r.Context(), identity, event)
	decision := queueEventDecision{}
	if granted {
		decision.Lease = &lease
	}
	openai.WriteJSON(w, http.StatusOK, decision)
}

func derefLease(lease *offloadLease) offloadLease {
	if lease == nil {
		return offloadLease{}
	}
	return *lease
}
