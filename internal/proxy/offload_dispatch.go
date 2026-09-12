package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/schedulingcost"
)

type queueAdmission struct {
	entry   *offloadEntry
	outcome offloadOutcome
}

func (service *Service) queueForLane(lane string) *offloadQueue {
	if lane == cluster.RouteLaneText {
		return service.textQueue
	}
	return service.imageQueue
}

func (service *Service) groupIDForLane(lane string, nodeID string, modelID string) (string, bool) {
	if service.registry == nil || service.queueForLane(lane) == nil {
		return "", false
	}
	groupID, members, ok := service.registry.GroupMembers(cluster.GroupMember{Lane: lane, NodeID: nodeID, ModelID: modelID})
	if !ok || len(members) < 2 {
		return "", false
	}
	return groupID, true
}

func (service *Service) imageGroupID(nodeID string, imageID string) (string, bool) {
	return service.groupIDForLane(cluster.RouteLaneImage, nodeID, imageID)
}

func (service *Service) textGroupID(nodeID string, modelID string) (string, bool) {
	return service.groupIDForLane(cluster.RouteLaneText, nodeID, modelID)
}

func (service *Service) enterQueue(ctx context.Context, lane string, groupID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	origin := nativeRequest
	if borrowed {
		origin = borrowedFromPeer
	}
	withdrawal := withdrawable
	if pinned {
		withdrawal = pinnedToThisNode
	}
	queue := service.queueForLane(lane)
	entry := queue.Enqueue(queuedRequest{
		groupID:         groupID,
		work:            work,
		requiredContext: requiredContext,
		origin:          origin,
		withdrawal:      withdrawal,
	}, service.nodeActivity(), time.Now())
	service.maybeOffload(lane, groupID)
	outcome, err := queue.Await(ctx, entry)
	if err != nil {
		return queueAdmission{}, err
	}
	return queueAdmission{entry: entry, outcome: outcome}, nil
}

func (service *Service) enterImageQueue(ctx context.Context, groupID string, work schedulingcost.Work, borrowed bool) (queueAdmission, error) {
	return service.enterQueue(ctx, cluster.RouteLaneImage, groupID, work, 0, false, borrowed)
}

func (service *Service) enterTextQueue(ctx context.Context, groupID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	return service.enterQueue(ctx, cluster.RouteLaneText, groupID, work, requiredContext, pinned, borrowed)
}

func (service *Service) completeQueueEntry(lane string, groupID string, entry *offloadEntry) {
	queue := service.queueForLane(lane)
	if queue == nil {
		return
	}
	queue.Complete(entry)
	service.maybeOffload(lane, groupID)
}

func (service *Service) completeImageQueueEntry(groupID string, entry *offloadEntry) {
	service.completeQueueEntry(cluster.RouteLaneImage, groupID, entry)
}

func (service *Service) completeTextQueueEntry(groupID string, entry *offloadEntry) {
	service.completeQueueEntry(cluster.RouteLaneText, groupID, entry)
}

func (service *Service) maybeOffload(lane string, groupID string) {
	queue := service.queueForLane(lane)
	if queue == nil || groupID == "" {
		return
	}
	key := backlogKey(lane, groupID)
	if _, live := service.activeOffloadLease(lane, groupID, time.Now()); !live {
		return
	}
	if _, busy := service.offloadInFlight.LoadOrStore(key, true); busy {
		return
	}
	withdrawn := queue.WithdrawNewest(groupID, 1)
	if len(withdrawn) == 0 {
		service.offloadInFlight.Delete(key)
	}
}

func (service *Service) finishOffload(lane string, groupID string) {
	service.offloadInFlight.Delete(backlogKey(lane, groupID))
	service.maybeOffload(lane, groupID)
}

func writeOffloadReturned(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "node has work of its own and returned this borrowed request")
}

func (service *Service) forwardOffloadedRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, lane string, groupID string, publicID string, release func()) bool {
	defer service.finishOffload(lane, groupID)

	response, err := service.sendOffloadedRequest(original.Context(), lane, groupID, forwarded, body)
	if err != nil {
		return false
	}
	response = responseWithRelease(response, release)
	if err := service.writeProxyResponse(w, response, publicID, true); err != nil {
		return true
	}
	return true
}

func (service *Service) forwardOffloadedImageRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, groupID string, publicImageID string, release func()) bool {
	return service.forwardOffloadedRequest(w, original, forwarded, body, cluster.RouteLaneImage, groupID, publicImageID, release)
}

func (service *Service) forwardOffloadedTextRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, groupID string, publicID string, release func()) bool {
	return service.forwardOffloadedRequest(w, original, forwarded, body, cluster.RouteLaneText, groupID, publicID, release)
}

func (service *Service) leasedHelperMember(lane string, groupID string, ownerNodeID string, ownerModelID string) (cluster.GroupMember, bool) {
	lease, ok := service.activeOffloadLease(lane, groupID, time.Now())
	if !ok {
		return cluster.GroupMember{}, false
	}
	_, members, ok := service.registry.GroupMembers(cluster.GroupMember{Lane: lane, NodeID: ownerNodeID, ModelID: ownerModelID})
	if !ok {
		return cluster.GroupMember{}, false
	}
	for _, member := range members {
		if member.NodeID == lease.HelperNodeID {
			return member, true
		}
	}
	return cluster.GroupMember{}, false
}

func (service *Service) leasedHelperContextFits(helper cluster.GroupMember, requiredContext int64) bool {
	if requiredContext <= 0 {
		return true
	}
	for _, model := range service.registry.Models() {
		if model.NodeID == helper.NodeID && model.LocalID == helper.ModelID {
			return cluster.ContextFits(model, int(requiredContext))
		}
	}
	return false
}
