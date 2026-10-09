package proxy

import (
	"encoding/json"
	"net/http"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteInventory(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	response, err := service.siteInventory(r.Context(), inventoryFilesRequested(r))
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeSiteInventory(w http.ResponseWriter, r *http.Request) {
	includeFiles := inventoryFilesRequested(r)
	if includeFiles {
		if err := service.refreshLocalRegistryWithLogs(); err != nil {
			openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
			return
		}
	}
	node, err := service.localNodeInventory(r.Context(), includeFiles)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, node)
}

func (service *Service) handleSiteCookPreview(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	var request siteapi.CookRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.planCook(r.Context(), request, true)
	if err != nil {
		if issues, ok := validationIssues(err); ok {
			openai.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "validation": issues})
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteCookApply(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	var request siteapi.CookRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.planCook(r.Context(), request, false)
	if err != nil {
		if issues, ok := validationIssues(err); ok {
			openai.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "validation": issues})
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteCookDelete(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	if service.recipeStore == nil {
		openai.WriteError(w, http.StatusBadRequest, "site_error", "recipe store is not configured")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/router/v1/site/cook/")
	if err := service.recipeStore.Delete(id); err != nil {
		openai.WriteError(w, http.StatusNotFound, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) handleNodeSiteConfigs(w http.ResponseWriter, r *http.Request) {
	var request cook.NodeConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := service.writeLocalCookConfig(r.Context(), request)
	if err != nil {
		if issues, ok := validationIssues(err); ok {
			openai.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "validation": issues})
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if !request.DryRun {
		if err := service.refreshLocalRegistry(); err != nil {
			openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
			return
		}
	}
	openai.WriteJSON(w, http.StatusOK, result)
}

func (service *Service) siteControlAllowed() bool {
	return service.clusterRole == cluster.RoleStandalone || service.clusterRole == cluster.RoleMaster
}
