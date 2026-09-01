package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/modelstate"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
	"tensors-router/internal/unloadpolicy"
)

const nodeSeparateRuntimesPath = "/router/v1/node/separate-runtimes"

func (service *Service) handleSiteSeparateRuntimes(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		service.writeSeparateRuntime(w, r, r.URL.Query().Get("node_id"), r.URL.Query().Get("local_id"), nil)
	case http.MethodPost:
		var request siteapi.SeparateRuntimeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		service.writeSeparateRuntime(w, r, request.NodeID, request.LocalID, &request.Settings)
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) handleNodeSeparateRuntimes(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
	localID := strings.TrimSpace(r.URL.Query().Get("local_id"))
	var settings *siteapi.SeparateRuntimeSettings
	if r.Method == http.MethodPost {
		var request siteapi.SeparateRuntimeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		nodeID, localID, settings = request.NodeID, request.LocalID, &request.Settings
	}
	if nodeID != "" && nodeID != service.nodeID {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("node %q was not found", nodeID))
		return
	}
	response, err := service.applyLocalSeparateRuntime(r.Context(), localID, settings)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "separate_runtime_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) writeSeparateRuntime(w http.ResponseWriter, r *http.Request, nodeID string, localID string, settings *siteapi.SeparateRuntimeSettings) {
	nodeID = strings.TrimSpace(nodeID)
	localID = strings.TrimSpace(localID)
	if localID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "local_id is required")
		return
	}
	if nodeID == "" || nodeID == service.nodeID {
		response, err := service.applyLocalSeparateRuntime(r.Context(), localID, settings)
		if err != nil {
			openai.WriteError(w, http.StatusBadGateway, "separate_runtime_error", err.Error())
			return
		}
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	nodeURL, ok := service.remoteNodeURL(nodeID)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("node %q was not found", nodeID))
		return
	}
	method, path, body := remoteSeparateRuntimeRequest(nodeID, localID, settings)
	var response siteapi.SeparateRuntimeResponse
	if err := service.clusterClient.JSON(r.Context(), method, nodeURL, path, body, &response); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "separate_runtime_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func remoteSeparateRuntimeRequest(nodeID string, localID string, settings *siteapi.SeparateRuntimeSettings) (method string, path string, body any) {
	if settings != nil {
		return http.MethodPost, nodeSeparateRuntimesPath, siteapi.SeparateRuntimeRequest{NodeID: nodeID, LocalID: localID, Settings: *settings}
	}
	statusQuery := url.Values{"node_id": {nodeID}, "local_id": {localID}}
	return http.MethodGet, nodeSeparateRuntimesPath + "?" + statusQuery.Encode(), nil
}

func (service *Service) applyLocalSeparateRuntime(ctx context.Context, localID string, settings *siteapi.SeparateRuntimeSettings) (siteapi.SeparateRuntimeResponse, error) {
	localID = strings.TrimSpace(localID)
	if localID == "" {
		return siteapi.SeparateRuntimeResponse{}, fmt.Errorf("local_id is required")
	}
	if service.modelStateStore == nil {
		return siteapi.SeparateRuntimeResponse{}, fmt.Errorf("model state store is not configured")
	}
	model, ok, err := service.catalog.Resolve(localID)
	if err != nil {
		return siteapi.SeparateRuntimeResponse{}, err
	}
	if !ok {
		return siteapi.SeparateRuntimeResponse{}, fmt.Errorf("model %q was not found", localID)
	}
	if settings != nil {
		resolved, err := unloadpolicy.ResolveSelection(unloadpolicy.Selection(settings.Triggers))
		if err != nil {
			return siteapi.SeparateRuntimeResponse{}, err
		}
		if err := service.modelStateStore.SetSeparateRuntime(ctx, model.ID, modelstate.SeparateRuntimeSettings{
			RunSeparate:    settings.RunSeparate,
			UnloadTriggers: []string(resolved),
		}); err != nil {
			return siteapi.SeparateRuntimeResponse{}, err
		}
		service.invalidateWebUIRoutes()
	}

	inherited := service.inheritedSeparateRuntime(model)
	response := siteapi.SeparateRuntimeResponse{
		NodeID:     service.nodeID,
		LocalID:    model.ID,
		Settings:   inherited,
		Inherited:  inherited,
		Candidates: service.separateRuntimeCandidates(model),
	}
	stored, present, err := service.modelStateStore.SeparateRuntime(ctx, model.ID)
	if err != nil {
		return siteapi.SeparateRuntimeResponse{}, err
	}
	if present {
		response.HasOverride = true
		response.Settings = siteapi.SeparateRuntimeSettings{RunSeparate: stored.RunSeparate, Triggers: stored.UnloadTriggers}
	}
	return response, nil
}

func (service *Service) inheritedSeparateRuntime(model catalog.Model) siteapi.SeparateRuntimeSettings {
	settings := siteapi.SeparateRuntimeSettings{Triggers: []string{}}
	if strings.TrimSpace(service.configDir) == "" {
		return settings
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, model.Filename))
	if err != nil {
		return settings
	}
	settings.RunSeparate = metadata.RunEmbedSeparate
	if resolved, err := unloadpolicy.ResolveSelection(metadata.RouterUnloadPolicy); err == nil {
		settings.Triggers = []string(resolved)
	}
	return settings
}

func (service *Service) separateRuntimeCandidates(anchor catalog.Model) siteapi.SeparateRuntimeCandidates {
	candidates := siteapi.SeparateRuntimeCandidates{
		Lanes:    []string{unloadpolicy.Text, unloadpolicy.Image, unloadpolicy.Embeddings, unloadpolicy.Voice, unloadpolicy.Music},
		Families: unloadpolicy.FamilyTriggerValues(),
		Configs:  []string{},
	}
	models, err := service.catalog.List()
	if err != nil {
		return candidates
	}
	seen := map[string]struct{}{}
	for _, model := range models {
		if model.ID == anchor.ID {
			continue
		}
		trigger := unloadpolicy.ConfigPrefix + model.ID
		if _, ok := seen[trigger]; ok {
			continue
		}
		seen[trigger] = struct{}{}
		candidates.Configs = append(candidates.Configs, trigger)
	}
	sort.Strings(candidates.Configs)
	return candidates
}
