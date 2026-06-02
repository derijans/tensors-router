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
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/models":
		service.handleRouterModels(w)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/models":
		if service.requireClusterToken(w, r) {
			service.handleNodeModels(w)
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
		"data":   cluster.LocalModels(models, "local", "", cluster.SourceLocal),
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
		Models: cluster.LocalModels(models, "local", "", cluster.SourceLocal),
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
	if err := service.registry.UpdateNode(snapshot); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	if service.registry != nil && service.registry.HasModel(publicID) {
		route, release, ok := service.registry.Acquire(publicID, service.backend.Healthy(ctx))
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
		route, release, ok := service.registry.Acquire(publicID, service.backend.Healthy(ctx))
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
	readiness := readinessText
	if model.HasImage && !model.HasLLM && !model.HasEmbeddings {
		readiness = readinessImage
	}
	release, _, err := service.acquireModelConfig(ctx, publicID, model.Filename, readiness, false)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (service *Service) unloadLocal(ctx context.Context) error {
	service.activeConfigMu.Lock()
	service.activeConfigFilename = ""
	service.notifyActiveConfigLocked()
	service.activeConfigMu.Unlock()
	return service.backend.Unload(ctx)
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
