package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/schedulingcost"
)

// queueAdmission is the outcome of waiting for a turn at the local backend.
// Exactly one of its fields is meaningful, matching the three ways a queued
// request can leave the queue.
type queueAdmission struct {
	entry    *offloadEntry
	admitted bool
	offload  bool
	returned bool
}

func (service *Service) queueForLane(lane string) *offloadQueue {
	if lane == cluster.RouteLaneText {
		return service.textQueue
	}
	return service.imageQueue
}

// groupIDForLane reports the routing group a locally served request belongs to
// in the given lane. A model in no group is not queued at all and behaves
// exactly as before.
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

// enterQueue holds the request until the backend has room, or until it is
// taken back to be lent to a peer, or until this node has to hand it back
// because work of its own arrived. nodeIdle is read fresh here, immediately
// before the request tries to land, rather than from the last published
// status, so a native request that arrived a moment ago on any runtime — not
// only this lane's queue — is never raced past.
func (service *Service) enterQueue(ctx context.Context, lane string, groupID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	queue := service.queueForLane(lane)
	entry := queue.Enqueue(groupID, work, requiredContext, borrowed, pinned, service.idleForBorrowedWork(), time.Now())
	service.maybeOffload(lane, groupID)
	outcome, err := queue.Await(ctx, entry)
	if err != nil {
		return queueAdmission{}, err
	}
	switch outcome {
	case offloadAdmitted:
		return queueAdmission{entry: entry, admitted: true}, nil
	case offloadWithdrawn:
		return queueAdmission{entry: entry, offload: true}, nil
	default:
		return queueAdmission{entry: entry, returned: true}, nil
	}
}

func (service *Service) enterImageQueue(ctx context.Context, groupID string, work schedulingcost.Work, borrowed bool) (queueAdmission, error) {
	return service.enterQueue(ctx, cluster.RouteLaneImage, groupID, work, 0, false, borrowed)
}

// enterTextQueue additionally carries requiredContext, for the per-request
// re-check before a withdrawn entry is actually handed to a helper, and
// pinned, which keeps a streaming request counted in the node's backlog
// without ever letting it be withdrawn — the router never buffered a
// replayable body for it.
func (service *Service) enterTextQueue(ctx context.Context, groupID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	return service.enterQueue(ctx, cluster.RouteLaneText, groupID, work, requiredContext, pinned, borrowed)
}

// completeQueueEntry frees the backend slot and immediately reconsiders
// lending, because a completion is exactly when a helper slot may become
// useful.
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

// maybeOffload withdraws at most one pending request for a group when a live
// lease says a peer can finish it sooner. One at a time is the whole
// discipline: a helper never holds a borrowed queue, so it has nothing to hand
// back beyond the job it is running, and the next request is sent only once
// that one completes. Keyed by (lane, groupID) so an image group and a text
// group that happen to share a group id string never contend for the same
// in-flight slot.
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

// writeOffloadReturned answers a borrowed request this node will not start. It is
// not a failure: the owner still holds the client and simply runs the request
// itself.
func writeOffloadReturned(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "node has work of its own and returned this borrowed request")
}

// forwardOffloadedRequest sends one withdrawn request to the master for
// placement on the leased helper and relays the answer to the client this node
// is still holding. It reports false when the helper could not take it, which
// is not a failure: the caller re-queues and runs the request itself.
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

// Routing groups link checkpoints that carry different ids on different
// nodes, so a relayed request must be addressed to the helper's own id — the
// helper's registry has never heard of the owner's.
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

// planOffloadLeases gates a lease on the owner's *average* pending request,
// so one long request in a queue of short ones can still be withdrawn toward
// a helper whose window cannot actually hold it — this is the per-entry
// re-check that closes that gap. A lane with no notion of context (image) or
// an entry that was never sized reports true: there is nothing to gate.
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
