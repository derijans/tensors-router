package proxy

import (
	"sort"
	"time"

	"tensors-router/internal/siteapi"
)

func (scheduler *scheduler) heldRequests(now time.Time) []siteapi.NodeHeldRequest {
	requests := make([]siteapi.NodeHeldRequest, 0)
	arrivals := make([]time.Time, 0)
	for _, lane := range lendingLanes {
		for _, held := range scheduler.queueForLane(lane).HeldSnapshot() {
			requests = append(requests, siteapi.NodeHeldRequest{Lane: lane, ModelID: held.modelID, State: siteapi.HeldRequestHeld})
			arrivals = append(arrivals, held.arrived)
		}
	}
	for _, lent := range scheduler.lent.Snapshot() {
		requests = append(requests, siteapi.NodeHeldRequest{
			Lane:          lent.lane,
			ModelID:       lent.modelID,
			State:         siteapi.HeldRequestLent,
			HelperNodeID:  lent.helperNodeID,
			HelperModelID: lent.helperModelID,
		})
		arrivals = append(arrivals, lent.arrived)
	}
	for index := range requests {
		requests[index].WaitingMS = now.Sub(arrivals[index]).Milliseconds()
	}
	sort.SliceStable(requests, func(left, right int) bool { return requests[left].WaitingMS > requests[right].WaitingMS })
	return requests
}
