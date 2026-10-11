package proxy

import (
	"context"
	"net/http"
)

func (service *Service) helperRunsBorrowedWork(ctx context.Context, helperNodeID string) bool {
	nodeURL := service.registry.NodeURLsByID()[helperNodeID]
	if nodeURL == "" {
		return false
	}
	var status NodeRuntimeStatus
	if err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/runtime-status", nil, &status); err != nil {
		return false
	}
	return status.BorrowedInFlight == nil || *status.BorrowedInFlight > 0
}

func (scheduler *scheduler) logStalledHelper(helperNodeID string, reclaimed int) {
	scheduler.logger.Printf("offload helper stalled helper=%s reclaimed=%d", helperNodeID, reclaimed)
}
