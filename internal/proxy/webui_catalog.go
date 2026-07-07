package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

type WebUICatalogResponse struct {
	Object string       `json:"object"`
	Data   []WebUIEntry `json:"data"`
}

type WebUIEntry struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Backend             string                 `json:"backend"`
	BackendMode         string                 `json:"backend_mode"`
	Lane                string                 `json:"lane"`
	URL                 string                 `json:"url"`
	NodeID              string                 `json:"node_id"`
	NodeURL             string                 `json:"node_url,omitempty"`
	Enabled             bool                   `json:"enabled"`
	Active              bool                   `json:"active"`
	ActiveModelID       string                 `json:"active_model_id,omitempty"`
	ActiveImageID       string                 `json:"active_image_id,omitempty"`
	RequiresLoadedModel bool                   `json:"requires_loaded_model"`
	CanOpenWithoutModel bool                   `json:"can_open_without_model"`
	CompatibleModels    []WebUICompatibleModel `json:"compatible_models"`
}

type WebUICompatibleModel struct {
	ID           string `json:"id"`
	ModelID      string `json:"model_id"`
	LocalID      string `json:"local_id,omitempty"`
	ImageID      string `json:"image_id,omitempty"`
	LocalImageID string `json:"local_image_id,omitempty"`
	Filename     string `json:"filename"`
	NodeID       string `json:"node_id"`
	NodeURL      string `json:"node_url,omitempty"`
	BackendMode  string `json:"backend_mode"`
	Active       bool   `json:"active"`
}

type webUIDefinition struct {
	kind        string
	name        string
	backend     string
	backendMode string
	lane        string
	path        string
}

type webUISession struct {
	mu      sync.RWMutex
	enabled map[string]bool
}

type webUISessionRequest struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type webUILoadRequest struct {
	ID      string `json:"id"`
	ModelID string `json:"model_id"`
	ImageID string `json:"image_id"`
}

type webUILoadResponse struct {
	OK      bool   `json:"ok"`
	ID      string `json:"id"`
	URL     string `json:"url"`
	ModelID string `json:"model_id,omitempty"`
	ImageID string `json:"image_id,omitempty"`
}

var webUIDefinitions = []webUIDefinition{
	{
		kind:        "kobold-lite",
		name:        "KoboldCpp Lite",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		lane:        cluster.RouteLaneText,
		path:        "/",
	},
	{
		kind:        "kobold-lcpp",
		name:        "KoboldCpp llama UI",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		lane:        cluster.RouteLaneText,
		path:        "/lcpp/",
	},
	{
		kind:        "kobold-sd",
		name:        "KoboldCpp StableUI",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		lane:        cluster.RouteLaneImage,
		path:        "/sdui/",
	},
	{
		kind:        "kobold-music",
		name:        "KoboldCpp MusicUI",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		lane:        cluster.RouteLaneMusic,
		path:        "/musicui/",
	},
	{
		kind:        "llama",
		name:        "llama-server UI",
		backend:     "llama.cpp",
		backendMode: BackendModeLlamaSDCPP,
		lane:        cluster.RouteLaneText,
		path:        "/",
	},
	{
		kind:        "sdcpp",
		name:        "sd-server UI",
		backend:     "stable-diffusion.cpp",
		backendMode: BackendModeLlamaSDCPP,
		lane:        cluster.RouteLaneImage,
		path:        "/",
	},
}

func newWebUISession() *webUISession {
	return &webUISession{enabled: map[string]bool{}}
}

func (session *webUISession) set(id string, enabled bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if enabled {
		session.enabled[id] = true
		return
	}
	delete(session.enabled, id)
}

func (session *webUISession) isEnabled(id string) bool {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.enabled[id]
}

func (session *webUISession) apply(entries []WebUIEntry) {
	session.mu.RLock()
	defer session.mu.RUnlock()
	for index := range entries {
		entries[index].Enabled = session.enabled[entries[index].ID]
	}
}

func (service *Service) handleSiteWebUIs(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	response, err := service.siteWebUIs(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeSiteWebUIs(w http.ResponseWriter, r *http.Request) {
	entries, err := service.localWebUIs()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, webUICatalogResponse(entries))
}

func (service *Service) handleSiteWebUISession(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request webUISessionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	request.ID = strings.TrimSpace(request.ID)
	response, err := service.siteWebUIs(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	if _, ok := webUIEntryByID(response.Data, request.ID); !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "webui not found")
		return
	}
	service.webUISession.set(request.ID, request.Enabled)
	response, err = service.siteWebUIs(r.Context())
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteWebUILoad(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, err := readWebUILoadRequest(r)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()
	response, err := service.loadSiteWebUI(ctx, request)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeSiteWebUILoad(w http.ResponseWriter, r *http.Request) {
	request, err := readWebUILoadRequest(r)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()
	response, err := service.loadLocalWebUI(ctx, request)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) siteWebUIs(ctx context.Context) (WebUICatalogResponse, error) {
	models, err := service.webUICatalogModels()
	if err != nil {
		return WebUICatalogResponse{}, err
	}
	entries := service.webUIsFromModels(models)
	service.markRemoteActiveWebUIs(ctx, entries)
	service.webUISession.apply(entries)
	sortWebUIEntries(entries)
	return webUICatalogResponse(entries), nil
}

func (service *Service) localWebUIs() ([]WebUIEntry, error) {
	models, err := service.localClusterModels()
	if err != nil {
		return nil, err
	}
	entries := service.webUIsFromModels(models)
	sortWebUIEntries(entries)
	return entries, nil
}

func (service *Service) webUICatalogModels() ([]cluster.Model, error) {
	if service.registry != nil {
		return service.siteModels(), nil
	}
	return service.localClusterModels()
}

func (service *Service) webUIsFromModels(models []cluster.Model) []WebUIEntry {
	entries := make([]WebUIEntry, 0, len(webUIDefinitions))
	for _, definition := range webUIDefinitions {
		compatibleModels := service.compatibleWebUIModels(definition, models)
		if len(compatibleModels) == 0 {
			continue
		}
		entry := WebUIEntry{
			ID:                  webUIEntryID(service.nodeID, definition.kind),
			Name:                definition.name,
			Backend:             definition.backend,
			BackendMode:         definition.backendMode,
			Lane:                definition.lane,
			URL:                 routerWebUIURL(definition),
			NodeID:              service.nodeID,
			NodeURL:             service.nodeURL,
			RequiresLoadedModel: true,
			CanOpenWithoutModel: false,
			CompatibleModels:    compatibleModels,
		}
		service.markActiveWebUIEntry(definition, &entry)
		entries = append(entries, entry)
	}
	return entries
}

func (service *Service) compatibleWebUIModels(definition webUIDefinition, models []cluster.Model) []WebUICompatibleModel {
	compatible := make([]WebUICompatibleModel, 0, len(models))
	for _, model := range models {
		modelBackendMode, err := service.resolveBackendMode(model.BackendMode)
		if err != nil || modelBackendMode != definition.backendMode {
			continue
		}
		if !webUIModelSupportsLane(definition, model) {
			continue
		}
		modelID := firstNonEmpty(model.PublicID, model.LocalID)
		imageID := firstNonEmpty(model.PublicImageID, model.ImageID)
		id := modelID
		if definition.lane == cluster.RouteLaneImage && imageID != "" {
			id = imageID
		}
		compatible = append(compatible, WebUICompatibleModel{
			ID:           id,
			ModelID:      modelID,
			LocalID:      model.LocalID,
			ImageID:      imageID,
			LocalImageID: model.ImageID,
			Filename:     model.Filename,
			NodeID:       model.NodeID,
			NodeURL:      model.NodeURL,
			BackendMode:  modelBackendMode,
		})
	}
	sort.Slice(compatible, func(left, right int) bool {
		return compatible[left].ID < compatible[right].ID
	})
	return compatible
}

func (service *Service) markActiveWebUIEntry(definition webUIDefinition, entry *WebUIEntry) {
	activeFilename := service.webUIActiveConfigFilename(definition)
	if activeFilename == "" {
		return
	}
	for index := range entry.CompatibleModels {
		if entry.CompatibleModels[index].NodeID != service.nodeID {
			continue
		}
		if entry.CompatibleModels[index].Filename != activeFilename {
			continue
		}
		entry.CompatibleModels[index].Active = true
		entry.Active = true
		entry.ActiveModelID = entry.CompatibleModels[index].ModelID
		if definition.lane == cluster.RouteLaneImage {
			entry.ActiveImageID = entry.CompatibleModels[index].ImageID
		}
		return
	}
}

func (service *Service) webUIActiveConfigFilename(definition webUIDefinition) string {
	runtime, err := service.runtimeForBackendMode(definition.backendMode, webUIReadiness(definition.lane))
	if err != nil || runtime == nil {
		return ""
	}
	return currentRuntimeConfigFilename(runtime)
}

func (service *Service) markRemoteActiveWebUIs(ctx context.Context, entries []WebUIEntry) {
	if service.clusterRole != cluster.RoleMaster {
		return
	}
	entryByID := map[string]*WebUIEntry{}
	for index := range entries {
		entryByID[entries[index].ID] = &entries[index]
	}
	for _, nodeURL := range service.remoteInventoryURLs() {
		var remote WebUICatalogResponse
		if err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/site/webuis", nil, &remote); err != nil {
			continue
		}
		for _, remoteEntry := range remote.Data {
			entry := entryByID[remoteEntry.ID]
			if entry == nil || !remoteEntry.Active {
				continue
			}
			markRemoteActiveWebUIEntry(entry, remoteEntry)
		}
	}
}

func markRemoteActiveWebUIEntry(entry *WebUIEntry, remote WebUIEntry) {
	entry.Active = true
	for _, remoteModel := range remote.CompatibleModels {
		if !remoteModel.Active {
			continue
		}
		for index := range entry.CompatibleModels {
			model := &entry.CompatibleModels[index]
			if model.NodeID != remoteModel.NodeID || model.Filename != remoteModel.Filename {
				continue
			}
			model.Active = true
			entry.ActiveModelID = model.ModelID
			if entry.Lane == cluster.RouteLaneImage {
				entry.ActiveImageID = model.ImageID
			}
			return
		}
	}
}

func (service *Service) loadSiteWebUI(ctx context.Context, request webUILoadRequest) (webUILoadResponse, error) {
	catalogResponse, err := service.siteWebUIs(ctx)
	if err != nil {
		return webUILoadResponse{}, err
	}
	entry, ok := webUIEntryByID(catalogResponse.Data, request.ID)
	if !ok {
		return webUILoadResponse{}, fmt.Errorf("webui %q was not found", request.ID)
	}
	model, ok := webUICompatibleModelForRequest(entry, request)
	if !ok {
		return webUILoadResponse{}, fmt.Errorf("compatible model was not found for webui %q", request.ID)
	}
	if model.NodeID != "" && model.NodeID != service.nodeID {
		if strings.TrimSpace(model.NodeURL) == "" {
			return webUILoadResponse{}, fmt.Errorf("node url for webui model %q is required", model.ID)
		}
		var response webUILoadResponse
		remoteRequest := webUILoadRequest{
			ID:      entry.ID,
			ModelID: firstNonEmpty(model.LocalID, model.ModelID),
			ImageID: firstNonEmpty(model.LocalImageID, model.ImageID),
		}
		if err := service.clusterClient.JSON(ctx, http.MethodPost, model.NodeURL, "/router/v1/node/site/webuis/load", remoteRequest, &response); err != nil {
			return webUILoadResponse{}, err
		}
		response.URL = entry.URL
		return response, nil
	}
	return service.loadLocalWebUIEntry(ctx, entry, model)
}

func (service *Service) loadLocalWebUI(ctx context.Context, request webUILoadRequest) (webUILoadResponse, error) {
	entries, err := service.localWebUIs()
	if err != nil {
		return webUILoadResponse{}, err
	}
	entry, ok := webUIEntryByID(entries, request.ID)
	if !ok {
		return webUILoadResponse{}, fmt.Errorf("webui %q was not found", request.ID)
	}
	model, ok := webUICompatibleModelForRequest(entry, request)
	if !ok {
		return webUILoadResponse{}, fmt.Errorf("compatible model was not found for webui %q", request.ID)
	}
	return service.loadLocalWebUIEntry(ctx, entry, model)
}

func (service *Service) loadLocalWebUIEntry(ctx context.Context, entry WebUIEntry, model WebUICompatibleModel) (webUILoadResponse, error) {
	if err := service.loadLocalConfig(ctx, entry.BackendMode, webUILoadModelID(entry, model), model.Filename, webUIReadiness(entry.Lane)); err != nil {
		return webUILoadResponse{}, err
	}
	return webUILoadResponse{
		OK:      true,
		ID:      entry.ID,
		URL:     entry.URL,
		ModelID: model.ModelID,
		ImageID: model.ImageID,
	}, nil
}

func webUILoadModelID(entry WebUIEntry, model WebUICompatibleModel) string {
	if entry.Lane == cluster.RouteLaneImage && strings.TrimSpace(model.ImageID) != "" {
		return model.ImageID
	}
	return model.ModelID
}

func readWebUILoadRequest(r *http.Request) (webUILoadRequest, error) {
	defer r.Body.Close()
	var request webUILoadRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return webUILoadRequest{}, err
	}
	request.ID = strings.TrimSpace(request.ID)
	request.ModelID = strings.TrimSpace(request.ModelID)
	request.ImageID = strings.TrimSpace(request.ImageID)
	if request.ID == "" {
		return webUILoadRequest{}, fmt.Errorf("id is required")
	}
	return request, nil
}

func webUIEntryByID(entries []WebUIEntry, id string) (WebUIEntry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return WebUIEntry{}, false
}

func webUICompatibleModelForRequest(entry WebUIEntry, request webUILoadRequest) (WebUICompatibleModel, bool) {
	for _, model := range entry.CompatibleModels {
		if request.ModelID == "" && request.ImageID == "" && model.Active {
			return model, true
		}
		if request.ModelID != "" && request.ModelID != model.ModelID && request.ModelID != model.LocalID && request.ModelID != model.ID {
			continue
		}
		if request.ImageID != "" && request.ImageID != model.ImageID && request.ImageID != model.LocalImageID && request.ImageID != model.ID {
			continue
		}
		return model, true
	}
	if request.ModelID == "" && request.ImageID == "" && len(entry.CompatibleModels) == 1 {
		return entry.CompatibleModels[0], true
	}
	return WebUICompatibleModel{}, false
}

func webUIModelSupportsLane(definition webUIDefinition, model cluster.Model) bool {
	switch definition.lane {
	case cluster.RouteLaneImage:
		return model.HasImage
	case cluster.RouteLaneMusic:
		return definition.backendMode == BackendModeKobold && model.HasMusic
	default:
		return model.HasLLM || model.HasEmbeddings || model.HasMultimodal || (definition.backendMode == BackendModeKobold && model.HasVoice)
	}
}

func webUIReadiness(lane string) backendReadiness {
	if lane == cluster.RouteLaneImage {
		return readinessImage
	}
	return readinessText
}

func webUICatalogResponse(entries []WebUIEntry) WebUICatalogResponse {
	return WebUICatalogResponse{
		Object: "list",
		Data:   entries,
	}
}

func webUIEntryID(nodeID string, kind string) string {
	return kind
}

func routerWebUIURL(definition webUIDefinition) string {
	return "/router/webuis/" + definition.kind + "/"
}

func webUIDefinitionByKind(kind string) (webUIDefinition, bool) {
	for _, definition := range webUIDefinitions {
		if definition.kind == kind {
			return definition, true
		}
	}
	return webUIDefinition{}, false
}

func sortWebUIEntries(entries []WebUIEntry) {
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].ID < entries[right].ID
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
