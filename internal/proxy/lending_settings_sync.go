package proxy

import (
	"context"
	"encoding/json"
	"net/http"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloadsettings"
	"tensors-router/internal/openai"
)

const nodeLendingSettingsPath = "/router/v1/node/offload/settings"

func lendingOverrideStoreOrNil(store *offloadsettings.Store) lendingOverrideStore {
	if store == nil {
		return nil
	}
	return store
}

func (service *Service) applyLendingSettings(settings offloadsettings.Settings) {
	service.scheduler.applySettings(settings)
	service.offloadDecisions.SetRetention(settings.DecisionRetention)
}

func (service *Service) reloadLendingSettings(ctx context.Context) {
	if service.clusterRole == cluster.RoleSlave {
		return
	}
	if _, err := service.lending.reload(ctx); err != nil {
		service.logger.Printf("lending settings load failed: %v", err)
	}
}

func (service *Service) publishLendingSettings(ctx context.Context) {
	if service.clusterRole != cluster.RoleMaster || service.registry == nil {
		return
	}
	values := service.lending.snapshot().Settings.Values()
	for nodeID, nodeURL := range service.registry.NodeURLsByID() {
		if nodeID == service.nodeID || nodeURL == "" {
			continue
		}
		err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, nodeLendingSettingsPath, values, nil)
		if !service.lending.deliveryFailures.outcomeChanged(nodeID, err) {
			continue
		}
		if err != nil {
			service.logger.Printf("lending settings delivery failed node=%s error=%v", nodeID, err)
		} else {
			service.logger.Printf("lending settings delivered node=%s", nodeID)
		}
	}
}

func (service *Service) handleNodeLendingSettings(w http.ResponseWriter, r *http.Request) {
	if service.clusterRole != cluster.RoleSlave {
		openai.WriteError(w, http.StatusConflict, "invalid_request_error", "only a slave accepts lending settings from its master")
		return
	}
	var values offloadsettings.Values
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.lending.adoptFromMaster(values); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
