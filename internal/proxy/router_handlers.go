package proxy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/buildinfo"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/unloadpolicy"
)

type modelControlRequest struct {
	Model  string `json:"model"`
	Target string `json:"target"`
}

func (service *Service) handleNodeInference(w http.ResponseWriter, r *http.Request) {
	path, ok := nodeInferencePath(r.URL.Path)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	forwarded := r.Clone(r.Context())
	forwardedURL := *r.URL
	forwardedURL.Path = path
	forwarded.URL = &forwardedURL
	forwarded.Header = r.Header.Clone()
	forwarded.Header.Del("Authorization")
	borrowed := strings.TrimSpace(forwarded.Header.Get(offloadMarkerHeader)) != ""
	forwarded.Header.Del(offloadMarkerHeader)
	restoreRequested := strings.TrimSpace(forwarded.Header.Get(offloadRestoreHeader)) != ""
	forwarded.Header.Del(offloadRestoreHeader)
	loadAllowed := strings.TrimSpace(forwarded.Header.Get(offloadLoadHeader)) != ""
	forwarded.Header.Del(offloadLoadHeader)
	if borrowed {
		forwarded = markBorrowedRequest(forwarded)
		if restoreRequested {
			forwarded = markBorrowRestoreRequested(forwarded)
		}
		if loadAllowed {
			forwarded = markBorrowLoadAllowed(forwarded)
		}
	}
	service.ServeHTTP(w, forwarded)
}

func nodeInferencePath(requestPath string) (string, bool) {
	path := strings.TrimPrefix(requestPath, "/router/v1/node/inference")
	if !isLocalInferencePath(path) {
		return "", false
	}
	return path, isTextPath(path) || isImagePath(path) || isVoicePath(path) || isMusicPath(path)
}

func isLocalInferencePath(path string) bool {
	return len(path) > 0 && path[0] == '/' && (len(path) == 1 || (path[1] != '/' && path[1] != '\\'))
}

func (service *Service) handleRouterModels(w http.ResponseWriter, _ *http.Request) {
	if service.registry != nil {
		openai.WriteJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   service.benchmarks.decorate(service.registry.Models()),
		})
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   service.benchmarks.decorate(cluster.LocalModelsWithBackendMode(models, "local", "", cluster.SourceLocal, service.backendMode)),
	})
}

func (service *Service) handleNodeModels(w http.ResponseWriter, r *http.Request) {
	if service.registry != nil {
		snapshot := service.registry.Snapshot()
		snapshot.Models = service.benchmarks.decorate(service.modelsWithRuntimeState(r.Context(), snapshot.Models))
		openai.WriteJSON(w, http.StatusOK, snapshot)
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, cluster.Snapshot{
		NodeID:                 "local",
		Models:                 service.benchmarks.decorate(cluster.LocalModelsWithBackendMode(models, "local", "", cluster.SourceLocal, service.backendMode)),
		ProtocolVersion:        cluster.ProtocolVersion,
		MinimumProtocolVersion: cluster.MinimumProtocolVersion,
		BuildVersion:           buildinfo.Current().Version,
	})
}

func (service *Service) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	if service.registry == nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", "cluster registry is not enabled")
		return
	}
	var snapshot cluster.Snapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.validateRegisteredNodeURL(snapshot.NodeURL); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	if err := service.registry.UpdateNode(snapshot); err != nil {
		switch cluster.ErrorCode(err) {
		case cluster.ErrorCodeDuplicateNode:
			openai.WriteErrorCode(w, http.StatusConflict, "cluster_error", cluster.ErrorCodeDuplicateNode, err.Error())
			return
		case cluster.ErrorCodeIncompatibleProtocol:
			service.logger.Printf("node registration refused, incompatible protocol: %v", err)
			openai.WriteErrorCode(w, http.StatusConflict, "cluster_error", cluster.ErrorCodeIncompatibleProtocol,
				fmt.Sprintf("%v; this master requires protocol version %d or newer, update the node", err, cluster.MinimumProtocolVersion))
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	if err := service.clusterClient.AllowBaseURLs(snapshot.NodeURL); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "cluster_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":                       true,
		"protocol_version":         cluster.ProtocolVersion,
		"minimum_protocol_version": cluster.MinimumProtocolVersion,
	})
}

func (service *Service) handleRouterVersion(w http.ResponseWriter, _ *http.Request) {
	openai.WriteJSON(w, http.StatusOK, map[string]any{
		"version":                  buildinfo.Current().Version,
		"protocol_version":         cluster.ProtocolVersion,
		"minimum_protocol_version": cluster.MinimumProtocolVersion,
	})
}

func (service *Service) validateRegisteredNodeURL(nodeURL string) error {
	nodeURL = strings.TrimSpace(nodeURL)
	if nodeURL == "" {
		return fmt.Errorf("node url is required")
	}
	if !configuredBaseURL(nodeURL, service.slaveURLs) {
		return fmt.Errorf("node url %q is not configured", nodeURL)
	}
	return nil
}

func configuredBaseURL(nodeURL string, configured []string) bool {
	for _, value := range configured {
		if cluster.BaseURLEqual(nodeURL, value) {
			return true
		}
	}
	return false
}

func (service *Service) handleRouterLoad(w http.ResponseWriter, r *http.Request) {
	if service.rejectModelLoadWhileDraining(w) {
		return
	}
	control, err := readModelControlRequest(r, true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()

	if err := service.loadPublicModel(ctx, control.Model); err != nil {
		writeRouterLoadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeRouterLoadError(w http.ResponseWriter, err error) {
	diagnostic, ok := backenddiagnostic.FromError(err)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusBadGateway, struct {
		Error             openai.ErrorDetail           `json:"error"`
		BackendDiagnostic backenddiagnostic.Diagnostic `json:"backend_diagnostic"`
	}{
		Error:             openai.ErrorDetail{Message: err.Error(), Type: "backend_error"},
		BackendDiagnostic: diagnostic,
	})
}

func (service *Service) handleRouterUnload(w http.ResponseWriter, r *http.Request) {
	control, err := readModelControlRequest(r, false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()

	target, err := unloadpolicy.ResolveTarget(control.Target)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.unloadPublicModel(ctx, control.Model, target); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) handleRouterShutdown(w http.ResponseWriter, _ *http.Request) {
	if service.shutdown == nil {
		openai.WriteError(w, http.StatusForbidden, "shutdown_disabled", "router shutdown is disabled")
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go service.shutdown()
}

func (service *Service) loadPublicModel(ctx context.Context, publicID string) error {
	publicID = strings.TrimSpace(publicID)
	if handled, err := service.loadRecipe(ctx, publicID); handled || err != nil {
		return err
	}
	if service.registry != nil && service.registry.HasModel(publicID) {
		model, ok := service.registry.Model(publicID)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		modelBackendMode, err := service.clusterModelBackendMode(model)
		if err != nil {
			return err
		}
		readiness := modelControlReadiness(modelBackendMode, model.HasEmbeddings, model.HasVoice, model.VLLMTask)
		route, release, ok := service.acquireRegistryModelControlRoute(ctx, publicID, modelBackendMode, readiness)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		defer release()
		if route.Remote {
			return service.clusterClient.Load(ctx, route.NodeURL, route.LocalID)
		}
		return service.loadLocalModel(ctx, route.PublicID, route.LocalID)
	}
	return service.loadLocalModel(ctx, publicID, publicID)
}

func (service *Service) unloadPublicModel(ctx context.Context, publicID string, target string) error {
	publicID = strings.TrimSpace(publicID)
	if publicID != "" && service.registry != nil && service.registry.HasModel(publicID) {
		model, ok := service.registry.Model(publicID)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		modelBackendMode, err := service.clusterModelBackendMode(model)
		if err != nil {
			return err
		}
		readiness := modelControlReadiness(modelBackendMode, model.HasEmbeddings, model.HasVoice, model.VLLMTask)
		route, release, ok := service.acquireRegistryModelControlRoute(ctx, publicID, modelBackendMode, readiness)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		defer release()
		if route.Remote {
			return service.clusterClient.Unload(ctx, route.NodeURL, route.LocalID, target)
		}
	}
	return service.unloadLocal(ctx, target)
}

func (service *Service) loadLocalModel(ctx context.Context, publicID string, localID string) error {
	model, ok, err := service.catalog.Resolve(localID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("model %q was not found", publicID)
	}
	enabled, err := service.localModelEnabled(ctx, model.ID)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("model %q is disabled", publicID)
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return err
	}
	for _, readiness := range modelLoadReadinesses(modelBackendMode, model) {
		if err := service.loadLocalConfig(ctx, modelBackendMode, publicID, model.Filename, readiness); err != nil {
			return err
		}
	}
	return nil
}

func modelLoadReadinesses(mode string, model catalog.Model) []backendReadiness {
	if mode == BackendModeVLLM {
		return []backendReadiness{modelControlReadiness(mode, model.HasEmbeddings, model.HasVoice, model.VLLMTask)}
	}
	separateEmbeddings := model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate
	if mode != BackendModeLlamaSDCPP {
		readinesses := make([]backendReadiness, 0, 4)
		if modelHasExplicitTextAsset(model) || model.HasMultimodal || model.HasEmbeddings && !separateEmbeddings {
			readinesses = append(readinesses, readinessText)
		}
		if model.HasImage {
			readinesses = append(readinesses, readinessImage)
		}
		if model.HasVoice {
			readinesses = append(readinesses, readinessForVoiceModel(model, mode))
		}
		if model.HasMusic {
			readinesses = append(readinesses, readinessMusic)
		}
		if separateEmbeddings {
			readinesses = append(readinesses, readinessEmbeddings)
		}
		if len(readinesses) == 0 {
			return []backendReadiness{readinessText}
		}
		return readinesses
	}

	readinesses := make([]backendReadiness, 0, 3)
	if modelNeedsPrimaryTextRuntime(model) {
		readinesses = append(readinesses, readinessText)
	}
	if model.HasImage {
		readinesses = append(readinesses, readinessImage)
	}
	if separateEmbeddings {
		readinesses = append(readinesses, readinessEmbeddings)
	}
	if len(readinesses) == 0 {
		return []backendReadiness{readinessText}
	}
	return readinesses
}

func modelHasExplicitTextAsset(model catalog.Model) bool {
	for _, key := range []string{"model", "model_param", "draftmodel", "model_hash", "model_param_hash", "draftmodel_hash"} {
		value, found := model.Options[key]
		if !found {
			continue
		}
		var decoded any
		if json.Unmarshal(value, &decoded) != nil {
			continue
		}
		switch typed := decoded.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		case []any:
			if len(typed) > 0 {
				return true
			}
		}
	}
	return false
}

func modelControlReadiness(mode string, hasEmbeddings bool, hasVoice bool, task string) backendReadiness {
	if mode != BackendModeVLLM {
		if hasVoice {
			return readinessSpeech
		}
		return readinessText
	}
	if hasVoice || isVLLMSpeechTask(task) {
		return readinessTranscription
	}
	if hasEmbeddings {
		return readinessEmbeddings
	}
	return readinessText
}

func (service *Service) acquireRegistryModelControlRoute(ctx context.Context, publicID string, mode string, readiness backendReadiness) (cluster.Route, func(), bool) {
	healthy := service.localBackendAvailableForRoute(ctx, mode, readiness)
	switch readiness {
	case readinessEmbeddings:
		return service.registry.AcquireEmbedding(publicID, healthy)
	case readinessTranscription:
		return service.registry.AcquireVoice(publicID, healthy)
	default:
		return service.registry.Acquire(publicID, healthy)
	}
}

func (service *Service) loadLocalConfig(ctx context.Context, mode string, publicID string, filename string, readiness backendReadiness) error {
	_, release, _, err := service.acquireModelConfigForBackendMode(mode, ctx, publicID, filename, readiness, false)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (service *Service) loadLocalRuntimeForRequest(ctx context.Context, mode string, publicID string, filename string, readiness backendReadiness) error {
	modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(ctx), modelOperationTimeout)
	defer cancelModelContext()
	return service.loadLocalConfig(modelContext, mode, publicID, filename, readiness)
}

func (service *Service) unloadLocal(ctx context.Context, target string) error {
	family := service.backendFamilies[service.currentBackendMode()]
	if family == nil {
		return service.unloadSeparateLane(ctx, target)
	}
	runtimes, err := service.runtimesForUnloadTarget(family.mode, target)
	if err != nil {
		return err
	}
	if target == unloadpolicy.Embeddings {
		// Kobold and llama serve non-separate embeddings inline on the text runtime;
		// only a dedicated embeddings runtime (vLLM) and the pool should be unloaded.
		runtimes = distinctFromTextRuntime(family, runtimes)
	}
	if err := service.unloadRuntimes(ctx, runtimes); err != nil {
		return err
	}
	return service.unloadSeparateLane(ctx, target)
}

func distinctFromTextRuntime(family *backendFamily, runtimes []*backendRuntime) []*backendRuntime {
	filtered := make([]*backendRuntime, 0, len(runtimes))
	for _, runtime := range runtimes {
		if runtime != nil && runtime != family.textRuntime {
			filtered = append(filtered, runtime)
		}
	}
	return filtered
}

func readModelControlRequest(r *http.Request, requireModel bool) (modelControlRequest, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return modelControlRequest{}, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		if requireModel {
			return modelControlRequest{}, fmt.Errorf("model is required")
		}
		return modelControlRequest{}, nil
	}
	var control modelControlRequest
	if err := json.Unmarshal(body, &control); err != nil {
		return modelControlRequest{}, err
	}
	control.Model = strings.TrimSpace(control.Model)
	control.Target = strings.TrimSpace(control.Target)
	if requireModel && control.Model == "" {
		return modelControlRequest{}, fmt.Errorf("model is required")
	}
	return control, nil
}

func (service *Service) requireClusterToken(w http.ResponseWriter, r *http.Request) bool {
	if service.clusterToken == "" {
		openai.WriteError(w, http.StatusUnauthorized, "unauthorized", "cluster token is required")
		return false
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if !strings.HasPrefix(header, "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(service.clusterToken)) != 1 {
		openai.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid cluster token")
		return false
	}
	return true
}
