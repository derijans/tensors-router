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

func (service *Service) queueForLane(lane string) *offloadQueue {
	if lane == cluster.RouteLaneText {
		return service.textQueue
	}
	return service.imageQueue
}

func (service *Service) queuesForLending(lane string, nodeID string, modelID string) bool {
	return service.queueForLane(lane) != nil &&
		service.routingLinkIndex().isLinked(lane, routinggroups.Endpoint{NodeID: nodeID, ModelID: modelID})
}

func (service *Service) enterQueue(ctx context.Context, lane string, modelID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
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
		modelID:         modelID,
		work:            work,
		requiredContext: requiredContext,
		origin:          origin,
		withdrawal:      withdrawal,
	}, service.nodeActivity(), time.Now())
	service.maybeOffload(lane, modelID)
	outcome, err := queue.Await(ctx, entry)
	if err != nil {
		return queueAdmission{}, err
	}
	return queueAdmission{entry: entry, outcome: outcome}, nil
}

func (service *Service) enterImageQueue(ctx context.Context, modelID string, work schedulingcost.Work, borrowed bool) (queueAdmission, error) {
	return service.enterQueue(ctx, cluster.RouteLaneImage, modelID, work, 0, false, borrowed)
}

func (service *Service) enterTextQueue(ctx context.Context, modelID string, work schedulingcost.Work, requiredContext int64, pinned bool, borrowed bool) (queueAdmission, error) {
	return service.enterQueue(ctx, cluster.RouteLaneText, modelID, work, requiredContext, pinned, borrowed)
}

func (service *Service) completeQueueEntry(lane string, modelID string, entry *offloadEntry) {
	queue := service.queueForLane(lane)
	if queue == nil {
		return
	}
	queue.Complete(entry)
	service.maybeOffload(lane, modelID)
}

func (service *Service) completeImageQueueEntry(modelID string, entry *offloadEntry) {
	service.completeQueueEntry(cluster.RouteLaneImage, modelID, entry)
}

func (service *Service) completeTextQueueEntry(modelID string, entry *offloadEntry) {
	service.completeQueueEntry(cluster.RouteLaneText, modelID, entry)
}

func (service *Service) maybeOffload(lane string, modelID string) {
	queue := service.queueForLane(lane)
	if queue == nil || modelID == "" {
		return
	}
	key := laneModelKey(lane, modelID)
	if _, live := service.activeOffloadLease(lane, modelID, time.Now()); !live {
		return
	}
	if _, busy := service.offloadInFlight.LoadOrStore(key, true); busy {
		return
	}
	withdrawn := queue.WithdrawNewest(modelID, 1)
	if len(withdrawn) == 0 {
		service.offloadInFlight.Delete(key)
	}
}

func (service *Service) finishOffload(lane string, modelID string) {
	service.offloadInFlight.Delete(laneModelKey(lane, modelID))
	service.maybeOffload(lane, modelID)
}

func writeOffloadReturned(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "node has work of its own and returned this borrowed request")
}

func (service *Service) forwardOffloadedRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, lane string, modelID string, publicID string, release func()) bool {
	defer service.finishOffload(lane, modelID)

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
