package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

type BackendWebUIHosts struct {
	Kobold string
	Llama  string
	SDCPP  string
}

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
	host        string
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

const (
	webUIHostKobold = "kobold"
	webUIHostLlama  = "llama"
	webUIHostSDCPP  = "sdcpp"
)

var webUIDefinitions = []webUIDefinition{
	{
		kind:        "kobold-lite",
		name:        "KoboldCpp Lite",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		host:        webUIHostKobold,
		lane:        cluster.RouteLaneText,
		path:        "/",
	},
	{
		kind:        "kobold-llama",
		name:        "KoboldCpp llama UI",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		host:        webUIHostKobold,
		lane:        cluster.RouteLaneText,
		path:        "/lcpp/",
	},
	{
		kind:        "kobold-stable",
		name:        "KoboldCpp StableUI",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		host:        webUIHostKobold,
		lane:        cluster.RouteLaneImage,
		path:        "/sdui/",
	},
	{
		kind:        "kobold-music",
		name:        "KoboldCpp MusicUI",
		backend:     "koboldcpp",
		backendMode: BackendModeKobold,
		host:        webUIHostKobold,
		lane:        cluster.RouteLaneMusic,
		path:        "/musicui/",
	},
	{
		kind:        "llama-server",
		name:        "llama-server UI",
		backend:     "llama.cpp",
		backendMode: BackendModeLlamaSDCPP,
		host:        webUIHostLlama,
		lane:        cluster.RouteLaneText,
		path:        "/",
	},
	{
		kind:        "sd-server",
		name:        "sd-server UI",
		backend:     "stable-diffusion.cpp",
		backendMode: BackendModeLlamaSDCPP,
		host:        webUIHostSDCPP,
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

func (session *webUISession) apply(entries []WebUIEntry) {
	session.mu.RLock()
	defer session.mu.RUnlock()
	for index := range entries {
		entries[index].Enabled = session.enabled[entries[index].ID]
	}
}

func webUIHostsFromConfig(hosts BackendWebUIHosts, families map[string]*backendFamily) BackendWebUIHosts {
	if strings.TrimSpace(hosts.Kobold) == "" {
		hosts.Kobold = backendFamilyTextURL(families[BackendModeKobold])
	}
	if strings.TrimSpace(hosts.Llama) == "" {
		hosts.Llama = backendFamilyTextURL(families[BackendModeLlamaSDCPP])
	}
	if strings.TrimSpace(hosts.SDCPP) == "" {
		hosts.SDCPP = backendFamilyImageURL(families[BackendModeLlamaSDCPP])
	}
	if strings.TrimSpace(hosts.SDCPP) == "" {
		hosts.SDCPP = hosts.Llama
	}
	return hosts
}

func backendFamilyTextURL(family *backendFamily) string {
	if family == nil || family.textRuntime == nil || family.textRuntime.backend == nil {
		return ""
	}
	return family.textRuntime.backend.URL().String()
}

func backendFamilyImageURL(family *backendFamily) string {
	if family == nil || family.imageRuntime == nil || family.imageRuntime.backend == nil {
		return ""
	}
	return family.imageRuntime.backend.URL().String()
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
	entries, err := service.localWebUIs()
	if err != nil {
		return WebUICatalogResponse{}, err
	}
	if service.clusterRole == cluster.RoleMaster {
		for _, nodeURL := range service.remoteInventoryURLs() {
			var remote WebUICatalogResponse
			if err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/site/webuis", nil, &remote); err != nil {
				continue
			}
			for index := range remote.Data {
				remote.Data[index].NodeURL = nodeURL
			}
			entries = append(entries, remote.Data...)
		}
	}
	service.webUISession.apply(entries)
	sortWebUIEntries(entries)
	return webUICatalogResponse(entries), nil
}

func (service *Service) localWebUIs() ([]WebUIEntry, error) {
	models, err := service.localClusterModels()
	if err != nil {
		return nil, err
	}
	entries := make([]WebUIEntry, 0, len(webUIDefinitions))
	for _, definition := range webUIDefinitions {
		hostURL := service.webUIHostURL(definition.host)
		if hostURL == "" {
			continue
		}
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
			URL:                 joinWebUIURL(hostURL, definition.path),
			NodeID:              service.nodeID,
			NodeURL:             service.nodeURL,
			RequiresLoadedModel: true,
			CanOpenWithoutModel: false,
			CompatibleModels:    compatibleModels,
		}
		service.markActiveWebUIEntry(definition, &entry)
		entries = append(entries, entry)
	}
	sortWebUIEntries(entries)
	return entries, nil
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
	if entry.NodeID != "" && entry.NodeID != service.nodeID {
		var response webUILoadResponse
		if err := service.clusterClient.JSON(ctx, http.MethodPost, entry.NodeURL, "/router/v1/node/site/webuis/load", request, &response); err != nil {
			return webUILoadResponse{}, err
		}
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
	if err := service.loadLocalConfigWithPolicy(ctx, entry.BackendMode, model.ID, model.Filename, webUIReadiness(entry.Lane), ""); err != nil {
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

func (service *Service) webUIHostURL(host string) string {
	switch host {
	case webUIHostKobold:
		return strings.TrimSpace(service.webUIHosts.Kobold)
	case webUIHostLlama:
		return strings.TrimSpace(service.webUIHosts.Llama)
	case webUIHostSDCPP:
		return strings.TrimSpace(service.webUIHosts.SDCPP)
	default:
		return ""
	}
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
	return firstNonEmpty(nodeID, "local") + ":" + kind
}

func joinWebUIURL(hostURL string, path string) string {
	parsed, err := url.Parse(strings.TrimSpace(hostURL))
	if err != nil {
		return ""
	}
	parsed.Path = joinPath(parsed.Path, path)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func sortWebUIEntries(entries []WebUIEntry) {
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].NodeID != entries[right].NodeID {
			return entries[left].NodeID < entries[right].NodeID
		}
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
