package proxy

import (
	"context"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
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
	scheduler.reportQueueEvent(lane, modelID, queueEventEnqueued)
	outcome, err := queue.Await(ctx, entry)
	if err != nil {
		return queueAdmission{}, err
	}
	scheduler.recordHelperQueueOutcome(lane, entry, outcome)
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
	scheduler.reportQueueEvent(lane, modelID, queueEventCompleted)
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
	scheduler.dispatchMu.Lock()
	defer scheduler.dispatchMu.Unlock()
	lease, live := scheduler.activeOffloadLease(lane, modelID, time.Now())
	if !live {
		return
	}
	queue := scheduler.queueForLane(lane)
	for scheduler.lent.Count(lane, modelID) < lease.slots() {
		withdrawn := queue.WithdrawNewest(modelID, 1)
		if len(withdrawn) == 0 {
			return
		}
		scheduler.lendWithdrawn(lease, withdrawn[0])
	}
}

func (scheduler *scheduler) lendWithdrawn(lease offloadLease, entry *offloadEntry) {
	scheduler.lent.Add(entry, lentRequest{
		lane:          lease.Lane,
		modelID:       lease.OwnerModelID,
		arrived:       entry.arrived,
		helperNodeID:  lease.HelperNodeID,
		helperModelID: lease.HelperModelID,
	})
	scheduler.record(offloaddecisions.Record{
		Kind:          offloaddecisions.KindDispatch,
		Lane:          lease.Lane,
		OwnerNodeID:   lease.OwnerNodeID,
		OwnerModelID:  lease.OwnerModelID,
		HelperNodeID:  lease.HelperNodeID,
		HelperModelID: lease.HelperModelID,
		Outcome:       offloaddecisions.OutcomeLent,
		Slots:         lease.slots(),
		LentOut:       scheduler.lent.Count(lease.Lane, lease.OwnerModelID),
		WaitMS:        time.Since(entry.arrived).Milliseconds(),
	})
}

func (scheduler *scheduler) finishOffload(lane string, modelID string, entry *offloadEntry, returned bool) {
	request, lent := scheduler.lent.Remove(entry)
	if lent && returned {
		scheduler.record(offloaddecisions.Record{
			Kind:          offloaddecisions.KindDispatch,
			Lane:          lane,
			OwnerModelID:  modelID,
			HelperNodeID:  request.helperNodeID,
			HelperModelID: request.helperModelID,
			Outcome:       offloaddecisions.OutcomeReturned,
			LentOut:       scheduler.lent.Count(lane, modelID),
		})
	}
	scheduler.reportQueueEvent(lane, modelID, queueEventBorrowedCompleted)
}

func writeOffloadReturned(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "node has work of its own and returned this borrowed request")
}

func (service *Service) forwardOffloadedRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, lane string, entry *offloadEntry, publicID string, release func()) bool {
	response, err := service.sendOffloadedRequest(original.Context(), lane, entry.modelID, forwarded, body)
	if err != nil {
		service.scheduler.finishOffload(lane, entry.modelID, entry, true)
		return false
	}
	defer service.scheduler.finishOffload(lane, entry.modelID, entry, false)
	response = responseWithRelease(response, release)
	_ = service.writeProxyResponse(w, response, publicID, true)
	return true
}

func (service *Service) forwardOffloadedImageRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, entry *offloadEntry, publicImageID string, release func()) bool {
	return service.forwardOffloadedRequest(w, original, forwarded, body, cluster.RouteLaneImage, entry, publicImageID, release)
}

func (service *Service) forwardOffloadedTextRequest(w http.ResponseWriter, original *http.Request, forwarded *http.Request, body []byte, entry *offloadEntry, publicID string, release func()) bool {
	return service.forwardOffloadedRequest(w, original, forwarded, body, cluster.RouteLaneText, entry, publicID, release)
}

func (service *Service) lentTextRequestFitsHelper(modelID string, entry *offloadEntry) bool {
	lease, leased := service.scheduler.activeOffloadLease(cluster.RouteLaneText, modelID, time.Now())
	return leased && service.leasedHelperContextFits(routinggroups.Endpoint{NodeID: lease.HelperNodeID, ModelID: lease.HelperModelID}, entry.requiredContext)
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
