package proxy

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

type NodeRuntimeStatus struct {
	NodeID                  string `json:"node_id"`
	BackendMode             string `json:"backend_mode"`
	ActiveSTTModelID        string `json:"active_stt_model_id,omitempty"`
	ActiveSTTConfigFilename string `json:"active_stt_config_filename,omitempty"`
	ActiveSTTBackendMode    string `json:"active_stt_backend_mode,omitempty"`
	ActiveRequests          int    `json:"active_requests"`
	QueuedRequests          int    `json:"queued_requests"`
}

type sttCandidate struct {
	model  cluster.Model
	status NodeRuntimeStatus
	local  bool
}

func (service *Service) localRuntimeStatus() NodeRuntimeStatus {
	mode := service.currentBackendMode()
	status := NodeRuntimeStatus{NodeID: service.nodeID, BackendMode: mode}
	family := service.backendFamilies[mode]
	for _, runtime := range uniqueBackendRuntimes(family) {
		runtime.state.mu.Lock()
		status.ActiveRequests += runtime.state.users
		status.QueuedRequests += runtime.state.switchWaiters
		runtime.state.mu.Unlock()
	}
	runtime, err := service.runtimeForBackendMode(mode, readinessTranscription)
	if err != nil || runtime == nil {
		return status
	}
	status.ActiveSTTConfigFilename = currentRuntimeConfigFilename(runtime)
	if status.ActiveSTTConfigFilename == "" {
		return status
	}
	status.ActiveSTTBackendMode = mode
	if service.catalog == nil {
		return status
	}
	if models, listErr := service.catalog.List(); listErr == nil {
		for _, model := range models {
			if model.Filename == status.ActiveSTTConfigFilename && catalogModelSupportsSTT(model) {
				status.ActiveSTTModelID = model.ID
				break
			}
		}
	}
	return status
}

func (service *Service) selectAutomaticSTTModel(ctx context.Context) (cluster.Model, bool) {
	models := service.sttSchedulingModels()
	if len(models) == 0 {
		return cluster.Model{}, false
	}
	localStatus := service.localRuntimeStatus()
	return service.selectAutomaticSTTCandidate(models, localStatus, service.remoteRuntimeStatuses(ctx))
}

func (service *Service) selectAutomaticSTTCandidate(models []cluster.Model, localStatus NodeRuntimeStatus, statuses map[string]NodeRuntimeStatus) (cluster.Model, bool) {
	for _, model := range models {
		if model.NodeID == service.nodeID && model.Filename == localStatus.ActiveSTTConfigFilename {
			return model, true
		}
	}
	candidates := make([]sttCandidate, 0, len(models))
	for _, model := range models {
		if model.NodeID == service.nodeID {
			candidates = append(candidates, sttCandidate{model: model, status: localStatus, local: true})
			continue
		}
		if status, ok := statuses[model.NodeID]; ok {
			candidates = append(candidates, sttCandidate{model: model, status: status})
		}
	}
	loadedRemote := filterSTTCandidates(candidates, func(candidate sttCandidate) bool {
		return !candidate.local && candidate.status.ActiveSTTConfigFilename == candidate.model.Filename
	})
	if selected, ok := service.chooseSTTCandidate(loadedRemote, func(candidate sttCandidate) int {
		return candidate.status.ActiveRequests + candidate.status.QueuedRequests
	}); ok {
		return selected.model, true
	}
	idle := filterSTTCandidates(candidates, func(candidate sttCandidate) bool {
		return candidate.status.ActiveRequests == 0 && candidate.status.QueuedRequests == 0
	})
	if selected, ok := service.chooseSTTCandidate(idle, func(sttCandidate) int { return 0 }); ok {
		return selected.model, true
	}
	if service.clusterRole == cluster.RoleMaster {
		for _, candidate := range candidates {
			if candidate.local {
				return candidate.model, true
			}
		}
	}
	remote := filterSTTCandidates(candidates, func(candidate sttCandidate) bool { return !candidate.local })
	if selected, ok := service.chooseSTTCandidate(remote, func(candidate sttCandidate) int {
		return candidate.status.QueuedRequests
	}); ok {
		return selected.model, true
	}
	return cluster.Model{}, false
}

func (service *Service) sttSchedulingModels() []cluster.Model {
	var models []cluster.Model
	if service.registry != nil {
		models = service.registry.Models()
	} else if service.catalog != nil {
		local, err := service.catalog.List()
		if err != nil {
			return nil
		}
		models = cluster.LocalModelsWithBackendMode(local, service.nodeID, service.nodeURL, "local", service.backendMode)
	}
	result := make([]cluster.Model, 0, len(models))
	for _, model := range models {
		if !model.Disabled && model.Available && clusterModelSupportsSTT(model) {
			result = append(result, model)
		}
	}
	return result
}

func catalogModelSupportsSTT(model catalog.Model) bool {
	return model.Capabilities.Voice != nil && strings.TrimSpace(model.Capabilities.Voice.WhisperModel) != "" || isVLLMSpeechTask(model.VLLMTask)
}

func clusterModelSupportsSTT(model cluster.Model) bool {
	return model.Capabilities.Voice != nil && strings.TrimSpace(model.Capabilities.Voice.WhisperModel) != "" || isVLLMSpeechTask(model.VLLMTask)
}

func isVLLMSpeechTask(task string) bool {
	switch strings.ToLower(strings.TrimSpace(task)) {
	case "speech", "transcription", "translation", "realtime":
		return true
	default:
		return false
	}
}

func (service *Service) remoteRuntimeStatuses(ctx context.Context) map[string]NodeRuntimeStatus {
	results := fanOutNodes(ctx, service.remoteInventoryURLs(), func(nodeContext context.Context, nodeURL string) (NodeRuntimeStatus, error) {
		var status NodeRuntimeStatus
		err := service.clusterClient.JSON(nodeContext, http.MethodGet, nodeURL, "/router/v1/node/runtime-status", nil, &status)
		return status, err
	})
	statuses := make(map[string]NodeRuntimeStatus)
	for _, result := range results {
		if result.Err == nil && result.Value.NodeID != "" {
			statuses[result.Value.NodeID] = result.Value
		}
	}
	return statuses
}

func filterSTTCandidates(values []sttCandidate, keep func(sttCandidate) bool) []sttCandidate {
	result := make([]sttCandidate, 0, len(values))
	for _, value := range values {
		if keep(value) {
			result = append(result, value)
		}
	}
	return result
}

func (service *Service) chooseSTTCandidate(values []sttCandidate, workload func(sttCandidate) int) (sttCandidate, bool) {
	if len(values) == 0 {
		return sttCandidate{}, false
	}
	sort.Slice(values, func(left, right int) bool {
		leftWork := workload(values[left])
		rightWork := workload(values[right])
		if leftWork != rightWork {
			return leftWork < rightWork
		}
		leftKey := values[left].model.NodeID + "|" + values[left].model.Filename
		rightKey := values[right].model.NodeID + "|" + values[right].model.Filename
		return leftKey < rightKey
	})
	minimum := workload(values[0])
	tied := 1
	for tied < len(values) && workload(values[tied]) == minimum {
		tied++
	}
	service.autoSTTMu.Lock()
	index := int(service.autoSTTNext % uint64(tied))
	service.autoSTTNext++
	service.autoSTTMu.Unlock()
	return values[index], true
}
