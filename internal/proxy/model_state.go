package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

var errModelStateNotFound = errors.New("model state target was not found")

func (service *Service) handleSiteModelState(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.ModelStateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	snapshot, err := service.setModelEnabled(r.Context(), request)
	if err != nil {
		writeModelStateError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, snapshot)
}

func (service *Service) handleNodeModelState(w http.ResponseWriter, r *http.Request) {
	var request siteapi.ModelStateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if nodeID := strings.TrimSpace(request.NodeID); nodeID != "" && nodeID != service.nodeID {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("node %q was not found", nodeID))
		return
	}
	snapshot, err := service.setLocalModelEnabled(r.Context(), request.LocalID, request.Enabled)
	if err != nil {
		writeModelStateError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, snapshot)
}

func (service *Service) setModelEnabled(ctx context.Context, request siteapi.ModelStateRequest) (cluster.Snapshot, error) {
	nodeID := strings.TrimSpace(request.NodeID)
	if nodeID == "" || nodeID == service.nodeID {
		return service.setLocalModelEnabled(ctx, request.LocalID, request.Enabled)
	}
	if service.registry == nil {
		return cluster.Snapshot{}, fmt.Errorf("%w: node %q", errModelStateNotFound, nodeID)
	}
	nodeURL := service.registry.NodeURLsByID()[nodeID]
	if nodeURL == "" {
		return cluster.Snapshot{}, fmt.Errorf("%w: node %q", errModelStateNotFound, nodeID)
	}
	snapshot, err := service.clusterClient.SetModelEnabled(ctx, nodeURL, cluster.ModelStateRequest{
		NodeID: nodeID, LocalID: request.LocalID, Enabled: request.Enabled,
	})
	if err != nil {
		return cluster.Snapshot{}, err
	}
	if strings.TrimSpace(snapshot.NodeID) != nodeID {
		return cluster.Snapshot{}, fmt.Errorf("node %q returned snapshot for %q", nodeID, snapshot.NodeID)
	}
	snapshot.NodeURL = nodeURL
	if err := service.registry.UpdateNode(snapshot); err != nil {
		return cluster.Snapshot{}, err
	}
	return snapshot, nil
}

func (service *Service) setLocalModelEnabled(ctx context.Context, localID string, enabled bool) (cluster.Snapshot, error) {
	localID = strings.TrimSpace(localID)
	model, ok, err := service.catalog.Resolve(localID)
	if err != nil {
		return cluster.Snapshot{}, err
	}
	if !ok {
		return cluster.Snapshot{}, fmt.Errorf("%w: model %q", errModelStateNotFound, localID)
	}
	if service.modelStateStore == nil {
		return cluster.Snapshot{}, fmt.Errorf("model state store is not configured")
	}
	wasDisabled, err := service.modelStateStore.Disabled(ctx, model.ID)
	if err != nil {
		return cluster.Snapshot{}, err
	}
	if err := service.modelStateStore.SetEnabled(ctx, model.ID, enabled); err != nil {
		return cluster.Snapshot{}, err
	}
	if err := service.refreshLocalRegistry(); err != nil {
		_ = service.modelStateStore.SetEnabled(context.Background(), model.ID, !wasDisabled)
		_ = service.refreshLocalRegistry()
		return cluster.Snapshot{}, err
	}
	if enabled {
		service.cancelPendingModelUnload(model.ID)
	} else {
		service.queueDisabledModelUnload(model)
	}
	if service.registry != nil {
		return service.registry.Snapshot(), nil
	}
	models, err := service.localClusterModels()
	if err != nil {
		return cluster.Snapshot{}, err
	}
	return cluster.Snapshot{NodeID: service.nodeID, NodeURL: service.nodeURL, Models: models}, nil
}

func writeModelStateError(w http.ResponseWriter, err error) {
	if errors.Is(err, errModelStateNotFound) {
		openai.WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	var remoteError *cluster.RemoteError
	if errors.As(err, &remoteError) && remoteError.StatusCode == http.StatusNotFound {
		openai.WriteError(w, http.StatusNotFound, "not_found", remoteError.Error())
		return
	}
	openai.WriteError(w, http.StatusBadGateway, "model_state_error", err.Error())
}

func (service *Service) localModelEnabled(ctx context.Context, localID string) (bool, error) {
	if service.modelStateStore == nil {
		return true, nil
	}
	disabled, err := service.modelStateStore.Disabled(ctx, localID)
	return !disabled, err
}

func (service *Service) cancelPendingModelUnload(localID string) {
	service.modelStateMu.Lock()
	if cancel := service.pendingModelUnloads[localID]; cancel != nil {
		cancel()
		delete(service.pendingModelUnloads, localID)
	}
	service.modelStateMu.Unlock()
}

func (service *Service) queueDisabledModelUnload(model catalog.Model) {
	service.modelStateMu.Lock()
	if cancel := service.pendingModelUnloads[model.ID]; cancel != nil {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	service.pendingModelUnloads[model.ID] = cancel
	service.modelStateMu.Unlock()
	go service.unloadDisabledModelWhenIdle(ctx, model)
}

func (service *Service) unloadDisabledModelWhenIdle(ctx context.Context, model catalog.Model) {
	mode, err := service.catalogModelBackendMode(model)
	if err != nil {
		service.logger.Printf("disabled model unload setup failed model=%q error=%v", model.ID, err)
		return
	}
	runtimes := map[*activeConfigState]*backendRuntime{}
	for _, readiness := range modelLoadReadinesses(mode, model) {
		runtime, runtimeErr := service.runtimeForBackendMode(mode, readiness)
		if runtimeErr != nil {
			service.logger.Printf("disabled model unload setup failed model=%q error=%v", model.ID, runtimeErr)
			return
		}
		runtimes[runtime.state] = runtime
	}
	for _, runtime := range runtimes {
		if err := service.unloadDisabledRuntime(ctx, runtime, model.ID, model.Filename); err != nil && !errors.Is(err, context.Canceled) {
			service.logger.Printf("disabled model unload failed model=%q config=%q backend=%q error=%v", model.ID, model.Filename, runtime.name, err)
		}
	}
}

func (service *Service) unloadDisabledRuntime(ctx context.Context, runtime *backendRuntime, localID string, filename string) error {
	state := runtime.state
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		enabled, err := service.localModelEnabled(ctx, localID)
		if err != nil || enabled {
			return err
		}
		state.mu.Lock()
		if state.filename != filename {
			state.mu.Unlock()
			return nil
		}
		if state.switching || state.users > 0 || state.switchWaiters > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return err
			}
			continue
		}
		select {
		case <-ctx.Done():
			state.mu.Unlock()
			return ctx.Err()
		default:
		}
		state.switching = true
		state.filename = ""
		clearPhysicalLoadProfileLocked(state)
		clearVRAMLoadStateLocked(state)
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		err = runtime.backend.Unload(ctx)
		state.mu.Lock()
		state.switching = false
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		service.invalidateWebUIRoutes()
		return err
	}
}
