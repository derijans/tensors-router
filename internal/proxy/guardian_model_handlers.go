package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/guardianmodels"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

const nodeGuardianModelsPath = "/router/v1/node/guardian-models"

type guardianSnapshot struct {
	Assignments []guardianmodels.Assignment `json:"assignments"`
}

func (service *Service) installStoredGuardianModels(ctx context.Context) ([]guardianmodels.Assignment, bool) {
	if service.guardians.store == nil {
		return nil, false
	}
	assignments, err := service.guardians.store.Assignments(ctx)
	if err != nil {
		service.logger.Printf("guardian model load failed: %v", err)
		return nil, false
	}
	service.guardians.install(assignments)
	return assignments, true
}

func (service *Service) publishGuardianModels(ctx context.Context) {
	if service.clusterRole != cluster.RoleMaster || service.registry == nil {
		return
	}
	snapshot := guardianSnapshot{Assignments: service.guardians.installed()}
	for nodeID, nodeURL := range service.registry.NodeURLsByID() {
		if nodeID == service.nodeID || nodeURL == "" {
			continue
		}
		if err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, nodeGuardianModelsPath, snapshot, nil); err != nil {
			service.logger.Printf("guardian model delivery failed node=%s error=%v", nodeID, err)
		}
	}
}

func (service *Service) handleNodeGuardianModels(w http.ResponseWriter, r *http.Request) {
	if service.clusterRole != cluster.RoleSlave {
		openai.WriteError(w, http.StatusConflict, "invalid_request_error", "only a slave accepts guardian models from its master")
		return
	}
	var snapshot guardianSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	service.guardians.install(snapshot.Assignments)
	w.WriteHeader(http.StatusNoContent)
}

func (service *Service) handleSiteGuardianModels(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.writeGuardianModels(w)
	case http.MethodPost:
		service.saveGuardianModel(w, r)
	case http.MethodDelete:
		service.deleteGuardianModel(w, r)
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) writeGuardianModels(w http.ResponseWriter) {
	assignments := service.guardians.installed()
	if assignments == nil {
		assignments = []guardianmodels.Assignment{}
	}
	openai.WriteJSON(w, http.StatusOK, siteapi.GuardianModelsResponse{Assignments: assignments, Candidates: service.guardianCandidates()})
}

func (service *Service) guardianCandidates() []siteapi.GuardianCandidate {
	candidates := []siteapi.GuardianCandidate{}
	if service.registry == nil {
		return candidates
	}
	for _, model := range service.registry.Models() {
		if guardianEligible(model) {
			candidates = append(candidates, siteapi.GuardianCandidate{NodeID: model.NodeID, ModelID: model.LocalID, Filename: model.Filename})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].NodeID != candidates[right].NodeID {
			return candidates[left].NodeID < candidates[right].NodeID
		}
		return candidates[left].ModelID < candidates[right].ModelID
	})
	return candidates
}

func guardianEligible(model cluster.Model) bool {
	return model.HasLLM && model.HasMultimodal && !model.Disabled
}

func (service *Service) saveGuardianModel(w http.ResponseWriter, r *http.Request) {
	if service.guardians.store == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "guardian models are not configured")
		return
	}
	var assignment guardianmodels.Assignment
	if err := json.NewDecoder(r.Body).Decode(&assignment); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.validateGuardianChoice(assignment.Guardian); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.guardians.store.Set(r.Context(), assignment); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	service.reloadAndPublishGuardianModels(r.Context())
	service.writeGuardianModels(w)
}

func (service *Service) validateGuardianChoice(guardian guardianmodels.Endpoint) error {
	if service.registry == nil {
		return nil
	}
	for _, model := range service.registry.Models() {
		if model.NodeID != guardian.NodeID || model.LocalID != guardian.ModelID {
			continue
		}
		if !guardianEligible(model) {
			return fmt.Errorf("%s is not a multimodal text model", guardian)
		}
		return nil
	}
	return fmt.Errorf("%s is not a known model", guardian)
}

func (service *Service) deleteGuardianModel(w http.ResponseWriter, r *http.Request) {
	if service.guardians.store == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "guardian models are not configured")
		return
	}
	query := r.URL.Query()
	scope := guardianmodels.Scope(strings.TrimSpace(query.Get("scope")))
	if err := service.guardians.store.Clear(r.Context(), scope, strings.TrimSpace(query.Get("node_id")), strings.TrimSpace(query.Get("model_id"))); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	service.reloadAndPublishGuardianModels(r.Context())
	service.writeGuardianModels(w)
}

func (service *Service) reloadAndPublishGuardianModels(ctx context.Context) {
	if _, loaded := service.installStoredGuardianModels(ctx); loaded {
		service.publishGuardianModels(ctx)
	}
}
