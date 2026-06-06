package proxy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

type modelControlRequest struct {
	Model string `json:"model"`
}

func (service *Service) handleRouterEndpoint(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/inventory":
		service.handleSiteInventory(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/cook/preview":
		service.handleSiteCookPreview(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/cook/apply":
		service.handleSiteCookApply(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/router/v1/site/cook/"):
		service.handleSiteCookDelete(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/config-file/preview":
		service.handleSiteConfigFilePreview(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/config-file/apply":
		service.handleSiteConfigFileApply(w, r)
	case r.Method == http.MethodDelete && r.URL.Path == "/router/v1/site/config-file":
		service.handleSiteConfigFileDelete(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/models":
		service.handleRouterModels(w)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/models":
		if service.requireClusterToken(w, r) {
			service.handleNodeModels(w)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/inventory":
		if service.requireClusterToken(w, r) {
			service.handleNodeSiteInventory(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/configs":
		if service.requireClusterToken(w, r) {
			service.handleNodeSiteConfigs(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/config-file/preview":
		if service.requireClusterToken(w, r) {
			service.handleNodeConfigFilePreview(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/config-file/apply":
		if service.requireClusterToken(w, r) {
			service.handleNodeConfigFileApply(w, r)
		}
	case r.Method == http.MethodDelete && r.URL.Path == "/router/v1/node/site/config-file":
		if service.requireClusterToken(w, r) {
			service.handleNodeConfigFileDelete(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/register":
		if service.requireClusterToken(w, r) {
			service.handleNodeRegister(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/load":
		service.handleRouterLoad(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/unload":
		service.handleRouterUnload(w, r)
	default:
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (service *Service) handleRouterModels(w http.ResponseWriter) {
	if service.registry != nil {
		openai.WriteJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   service.registry.Models(),
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
		"data":   cluster.LocalModelsWithBackendMode(models, "local", "", cluster.SourceLocal, service.backendMode),
	})
}

func (service *Service) handleNodeModels(w http.ResponseWriter) {
	if service.registry != nil {
		openai.WriteJSON(w, http.StatusOK, service.registry.Snapshot())
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, cluster.Snapshot{
		NodeID: "local",
		Models: cluster.LocalModelsWithBackendMode(models, "local", "", cluster.SourceLocal, service.backendMode),
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
	if err := service.allowRegisteredNodeURL(snapshot.NodeURL); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	if err := service.registry.UpdateNode(snapshot); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) allowRegisteredNodeURL(nodeURL string) error {
	nodeURL = strings.TrimSpace(nodeURL)
	if nodeURL == "" {
		return nil
	}
	if len(service.slaveURLs) > 0 && !configuredBaseURL(nodeURL, service.slaveURLs) {
		return fmt.Errorf("node url %q is not configured", nodeURL)
	}
	return service.clusterClient.AllowBaseURLs(nodeURL)
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
	control, err := readModelControlRequest(r, true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()

	if err := service.loadPublicModel(ctx, control.Model); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) handleRouterUnload(w http.ResponseWriter, r *http.Request) {
	control, err := readModelControlRequest(r, false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()

	if err := service.unloadPublicModel(ctx, control.Model); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) loadPublicModel(ctx context.Context, publicID string) error {
	publicID = strings.TrimSpace(publicID)
	if handled, err := service.loadRecipe(ctx, publicID); handled || err != nil {
		return err
	}
	if service.registry != nil && service.registry.HasModel(publicID) {
		route, release, ok := service.registry.Acquire(publicID, service.textRuntime.backend.Healthy(ctx))
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

func (service *Service) unloadPublicModel(ctx context.Context, publicID string) error {
	publicID = strings.TrimSpace(publicID)
	if publicID != "" && service.registry != nil && service.registry.HasModel(publicID) {
		route, release, ok := service.registry.Acquire(publicID, service.textRuntime.backend.Healthy(ctx))
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		defer release()
		if route.Remote {
			return service.clusterClient.Unload(ctx, route.NodeURL, route.LocalID)
		}
	}
	return service.unloadLocal(ctx)
}

func (service *Service) loadLocalModel(ctx context.Context, publicID string, localID string) error {
	model, ok, err := service.catalog.Resolve(localID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("model %q was not found", publicID)
	}
	if service.backendMode == BackendModeLlamaSDCPP && model.HasImage && (model.HasLLM || model.HasEmbeddings || model.HasMultimodal) {
		if err := service.loadLocalConfig(ctx, service.textRuntime, publicID, model.Filename, readinessText); err != nil {
			return err
		}
		return service.loadLocalConfig(ctx, service.imageRuntime, publicID, model.Filename, readinessImage)
	}

	readiness := readinessText
	runtime := service.textRuntime
	if model.HasImage && !model.HasLLM && !model.HasEmbeddings && !model.HasMultimodal {
		readiness = readinessImage
		runtime = service.imageRuntime
	}
	return service.loadLocalConfig(ctx, runtime, publicID, model.Filename, readiness)
}

func (service *Service) loadLocalConfig(ctx context.Context, runtime *backendRuntime, publicID string, filename string, readiness backendReadiness) error {
	release, _, err := service.acquireModelConfig(runtime, ctx, publicID, filename, readiness, false)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (service *Service) loadLocalRuntimeForRequest(ctx context.Context, runtime *backendRuntime, publicID string, filename string, readiness backendReadiness) error {
	modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(ctx), modelOperationTimeout)
	defer cancelModelContext()
	return service.loadLocalConfig(modelContext, runtime, publicID, filename, readiness)
}

func (service *Service) unloadLocal(ctx context.Context) error {
	if service.imageRuntime == service.textRuntime {
		return service.unloadRuntime(ctx, service.textRuntime)
	}

	errors := make(chan error, 2)
	go func() {
		errors <- service.unloadRuntime(ctx, service.textRuntime)
	}()
	go func() {
		errors <- service.unloadRuntime(ctx, service.imageRuntime)
	}()

	var firstErr error
	for index := 0; index < 2; index++ {
		if err := <-errors; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
