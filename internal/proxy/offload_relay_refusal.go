package proxy

import (
	"context"
	"errors"
	"net/http"

	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/openai"
)

const (
	relayRefusedNoLiveLease       = "no_live_lease"
	relayRefusedHelperUnreachable = "helper_unreachable"
	relayRefusedForwardFailed     = "forward_failed"
	relayRefusedHelperStalled     = "helper_stalled"
)

func relayRefusalReason(relayContext context.Context) string {
	if errors.Is(context.Cause(relayContext), errHelperStalled) {
		return relayRefusedHelperStalled
	}
	return relayRefusedForwardFailed
}

func (service *Service) refuseOffloadRelay(w http.ResponseWriter, lease offloadLease, reason string, detail string) {
	service.logger.Printf("offload relay refused lane=%s owner=%s/%q helper=%s/%q reason=%s detail=%s",
		lease.Lane, lease.OwnerNodeID, lease.OwnerModelID, lease.HelperNodeID, lease.HelperModelID, reason, detail)
	service.scheduler.record(offloaddecisions.Record{
		Kind:          offloaddecisions.KindDispatch,
		Lane:          lease.Lane,
		OwnerNodeID:   lease.OwnerNodeID,
		OwnerModelID:  lease.OwnerModelID,
		HelperNodeID:  lease.HelperNodeID,
		HelperModelID: lease.HelperModelID,
		Outcome:       offloaddecisions.OutcomeRelayRefused,
		Reason:        reason,
	})
	openai.WriteError(w, http.StatusConflict, offloadReturnedCode, detail)
}
