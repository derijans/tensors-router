package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

// imageQueueAdmission is the outcome of waiting for a turn at the local backend.
// Exactly one of its fields is meaningful, matching the three ways a queued image
// request can leave the queue.
type imageQueueAdmission struct {
	entry    *offloadEntry
	admitted bool
	offload  bool
	returned bool
}

// imageGroupID reports the routing group a locally served image request belongs
// to. A model in no group is not queued at all and behaves exactly as before.
func (service *Service) imageGroupID(nodeID string, imageID string) (string, bool) {
	if service.registry == nil || service.imageQueue == nil {
		return "", false
	}
	groupID, members, ok := service.registry.GroupMembers(cluster.GroupMember{NodeID: nodeID, ImageID: imageID})
	if !ok || len(members) < 2 {
		return "", false
	}
	return groupID, true
}

// enterImageQueue holds the request until the backend has room, or until it is
// taken back to be lent to a peer, or until this node has to hand it back because
// work of its own arrived.
func (service *Service) enterImageQueue(ctx context.Context, groupID string, work float64, borrowed bool) (imageQueueAdmission, error) {
	entry := service.imageQueue.Enqueue(groupID, work, borrowed, time.Now())
	service.maybeOffload(groupID)
	outcome, err := service.imageQueue.Await(ctx, entry)
	if err != nil {
		return imageQueueAdmission{}, err
	}
	switch outcome {
	case offloadAdmitted:
		return imageQueueAdmission{entry: entry, admitted: true}, nil
	case offloadWithdrawn:
		return imageQueueAdmission{entry: entry, offload: true}, nil
	default:
		return imageQueueAdmission{entry: entry, returned: true}, nil
	}
}

// completeImageQueueEntry frees the backend slot and immediately reconsiders
// lending, because a completion is exactly when a helper slot may become useful.
func (service *Service) completeImageQueueEntry(groupID string, entry *offloadEntry) {
	service.imageQueue.Complete(entry)
	service.maybeOffload(groupID)
}

// maybeOffload withdraws at most one pending request for a group when a live lease
// says a peer can finish it sooner. One at a time is the whole discipline: a helper
// never holds a borrowed queue, so it has nothing to hand back beyond the job it is
// running, and the next request is sent only once that one completes.
func (service *Service) maybeOffload(groupID string) {
	if service.imageQueue == nil || groupID == "" {
		return
	}
	if _, live := service.activeOffloadLease(groupID, time.Now()); !live {
		return
	}
	if _, busy := service.offloadInFlight.LoadOrStore(groupID, true); busy {
		return
	}
	withdrawn := service.imageQueue.WithdrawNewest(groupID, 1)
	if len(withdrawn) == 0 {
		service.offloadInFlight.Delete(groupID)
	}
}

func (service *Service) finishOffload(groupID string) {
	service.offloadInFlight.Delete(groupID)
	service.maybeOffload(groupID)
}

// writeOffloadReturned answers a borrowed request this node will not start. It is
// not a failure: the owner still holds the client and simply runs the request
// itself.
func writeOffloadReturned(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "node has work of its own and returned this borrowed request")
}

// forwardOffloadedImageRequest sends one withdrawn request to the master for
// placement on the leased helper and relays the answer to the client this node is
// still holding. It reports false when the helper could not take it, which is not
// a failure: the caller re-queues and runs the request itself.
func (service *Service) forwardOffloadedImageRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, groupID string, publicImageID string, release func()) bool {
	defer service.finishOffload(groupID)

	response, err := service.sendOffloadedRequest(original.Context(), groupID, forwarded, body)
	if err != nil {
		return false
	}
	response = responseWithRelease(response, release)
	if err := service.writeProxyResponse(w, response, publicImageID, true); err != nil {
		return true
	}
	return true
}
