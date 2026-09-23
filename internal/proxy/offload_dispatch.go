package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/schedulingcost"
)

type queueAdmission struct {
	entry   *offloadEntry
	outcome offloadOutcome
}

func (scheduler *scheduler) queuesForLending(lane string, nodeID string, modelID string) bool {
	return scheduler.deps.routingLinkIndex().isLinked(lane, routinggroups.Endpoint{NodeID: nodeID, ModelID: modelID})
}

func (scheduler *scheduler) enterQueue(ctx context.Context, lane string, modelID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	origin := nativeRequest
	if borrowed {
		origin = borrowedFromPeer
	}
	withdrawal := withdrawable
	if pinned {
		withdrawal = pinnedToThisNode
	}
	queue := scheduler.queueForLane(lane)
	entry := queue.Enqueue(queuedRequest{
		modelID:         modelID,
		work:            work,
		requiredContext: requiredContext,
		origin:          origin,
		withdrawal:      withdrawal,
	}, scheduler.nodeActivity(), time.Now())
	scheduler.maybeOffload(lane, modelID)
	outcome, err := queue.Await(ctx, entry)
	if err != nil {
		return queueAdmission{}, err
	}
	return queueAdmission{entry: entry, outcome: outcome}, nil
}

func (scheduler *scheduler) enterImageQueue(ctx context.Context, modelID string, work schedulingcost.Work, borrowed bool) (queueAdmission, error) {
	return scheduler.enterQueue(ctx, cluster.RouteLaneImage, modelID, work, 0, false, borrowed)
}

func (scheduler *scheduler) enterTextQueue(ctx context.Context, modelID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	return scheduler.enterQueue(ctx, cluster.RouteLaneText, modelID, work, requiredContext, pinned, borrowed)
}

func (scheduler *scheduler) completeQueueEntry(lane string, modelID string, entry *offloadEntry) {
	scheduler.queueForLane(lane).Complete(entry)
	scheduler.maybeOffload(lane, modelID)
}

func (scheduler *scheduler) completeImageQueueEntry(modelID string, entry *offloadEntry) {
	scheduler.completeQueueEntry(cluster.RouteLaneImage, modelID, entry)
}

func (scheduler *scheduler) completeTextQueueEntry(modelID string, entry *offloadEntry) {
	scheduler.completeQueueEntry(cluster.RouteLaneText, modelID, entry)
}

func (scheduler *scheduler) maybeOffload(lane string, modelID string) {
	if modelID == "" {
		return
	}
	key := laneModelKey(lane, modelID)
	if _, live := scheduler.activeOffloadLease(lane, modelID, time.Now()); !live {
		return
	}
	if _, busy := scheduler.offloadInFlight.LoadOrStore(key, true); busy {
		return
	}
	withdrawn := scheduler.queueForLane(lane).WithdrawNewest(modelID, 1)
	if len(withdrawn) == 0 {
		scheduler.offloadInFlight.Delete(key)
	}
}

func (scheduler *scheduler) finishOffload(lane string, modelID string) {
	scheduler.offloadInFlight.Delete(laneModelKey(lane, modelID))
	scheduler.maybeOffload(lane, modelID)
}

func writeOffloadReturned(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "node has work of its own and returned this borrowed request")
}

func (service *Service) forwardOffloadedRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, lane string, modelID string, publicID string, release func()) bool {
	defer service.scheduler.finishOffload(lane, modelID)

	response, err := service.sendOffloadedRequest(original.Context(), lane, modelID, forwarded, body)
	if err != nil {
		return false
	}
	response = responseWithRelease(response, release)
	if err := service.writeProxyResponse(w, response, publicID, true); err != nil {
		return true
	}
	return true
}

func (service *Service) forwardOffloadedImageRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, modelID string, publicImageID string, release func()) bool {
	return service.forwardOffloadedRequest(w, original, forwarded, body, cluster.RouteLaneImage, modelID, publicImageID, release)
}

func (service *Service) forwardOffloadedTextRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, modelID string, publicID string, release func()) bool {
	return service.forwardOffloadedRequest(w, original, forwarded, body, cluster.RouteLaneText, modelID, publicID, release)
}

func (service *Service) leasedHelperContextFits(helper routinggroups.Endpoint, requiredContext int64) bool {
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
