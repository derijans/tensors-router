package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
	"tensors-router/internal/vllm"
)

const (
	backendIDKoboldCPP   = "koboldcpp"
	backendIDLlamaServer = "llama-server"
	backendIDSDServer    = "sd-server"
	backendIDWhisper     = "whisper-server"
	backendIDVLLM        = "vllm"
)

var errRuntimeGenerationChanged = errors.New("runtime changed before unload")

type nodeBackendDefinition struct {
	id          string
	displayName string
	mode        string
}

type runtimeBinding struct {
	backendID string
	lane      string
	runtime   *backendRuntime
}

type activeRequestSnapshot struct {
	tag     uint64
	modelID string
}

var nodeBackendDefinitions = []nodeBackendDefinition{
	{id: backendIDKoboldCPP, displayName: "KoboldCpp", mode: backendmode.Kobold},
	{id: backendIDLlamaServer, displayName: "llama-server", mode: backendmode.LlamaSDCPP},
	{id: backendIDSDServer, displayName: "stable-diffusion.cpp", mode: backendmode.LlamaSDCPP},
	{id: backendIDWhisper, displayName: "whisper.cpp", mode: backendmode.LlamaSDCPP},
	{id: backendIDVLLM, displayName: "vLLM", mode: backendmode.VLLM},
}

func copyStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func (service *Service) handleSiteNodeState(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
	if nodeID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id is required")
		return
	}
	state, err := service.nodeState(r.Context(), nodeID)
	if err != nil {
		writeNodeStateError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, state)
}

func (service *Service) handleNodeState(w http.ResponseWriter) {
	openai.WriteJSON(w, http.StatusOK, service.localNodeState())
}

func (service *Service) handleSiteNodeUnload(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, ok := decodeNodeUnloadRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()
	if err := service.unloadNodeRuntime(ctx, request); err != nil {
		writeNodeStateError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (service *Service) handleNodeStateUnload(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeNodeUnloadRequest(w, r)
	if !ok {
		return
	}
	if request.NodeID != "" && request.NodeID != service.nodeID {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id does not match this node")
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()
	if err := service.unloadLocalRuntime(ctx, request); err != nil {
		writeNodeStateError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (service *Service) handleSiteBackendInitialization(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, ok := decodeBackendInitializationRequest(w, r)
	if !ok {
		return
	}
	job, err := service.initializeNodeBackend(r.Context(), request)
	if err != nil {
		writeBackendInitializationError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusAccepted, job)
}

func (service *Service) handleSiteBackendInitializationCancel(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, ok := decodeBackendInitializationRequest(w, r)
	if !ok {
		return
	}
	job, err := service.cancelNodeBackendInitialization(r.Context(), request)
	if err != nil {
		writeBackendInitializationError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, job)
}

func (service *Service) handleNodeBackendInitialization(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeBackendInitializationRequest(w, r)
	if !ok {
		return
	}
	if request.NodeID != service.nodeID {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id does not match this node")
		return
	}
	job, err := service.initializeLocalBackend(r.Context(), request)
	if err != nil {
		writeBackendInitializationError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusAccepted, job)
}

func (service *Service) handleNodeBackendInitializationCancel(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeBackendInitializationRequest(w, r)
	if !ok {
		return
	}
	if request.NodeID != service.nodeID {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id does not match this node")
		return
	}
	job, err := service.cancelLocalBackendInitialization(r.Context(), request)
	if err != nil {
		writeBackendInitializationError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, job)
}

func decodeBackendInitializationRequest(w http.ResponseWriter, r *http.Request) (siteapi.BackendInitializationRequest, bool) {
	defer r.Body.Close()
	var request siteapi.BackendInitializationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return siteapi.BackendInitializationRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body must contain one JSON object")
		return siteapi.BackendInitializationRequest{}, false
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
	request.BackendID = strings.TrimSpace(request.BackendID)
	request.Profile = strings.TrimSpace(request.Profile)
	if request.NodeID == "" || request.BackendID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id and backend_id are required")
		return siteapi.BackendInitializationRequest{}, false
	}
	if request.BackendID != backendIDVLLM {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "backend_id must be vllm")
		return siteapi.BackendInitializationRequest{}, false
	}
	if request.Profile != "" && !safeBackendProfile(request.Profile) {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "profile is invalid")
		return siteapi.BackendInitializationRequest{}, false
	}
	return request, true
}

func safeBackendProfile(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return value != ""
}

func (service *Service) initializeNodeBackend(ctx context.Context, request siteapi.BackendInitializationRequest) (siteapi.BackendInitializationJob, error) {
	if request.NodeID == service.nodeID {
		return service.initializeLocalBackend(ctx, request)
	}
	nodeURL, ok := service.remoteNodeURL(request.NodeID)
	if !ok {
		return siteapi.BackendInitializationJob{}, fmt.Errorf("unknown node %q", request.NodeID)
	}
	var job siteapi.BackendInitializationJob
	err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/backends/init", request, &job)
	return job, err
}

func (service *Service) cancelNodeBackendInitialization(ctx context.Context, request siteapi.BackendInitializationRequest) (siteapi.BackendInitializationJob, error) {
	if request.NodeID == service.nodeID {
		return service.cancelLocalBackendInitialization(ctx, request)
	}
	nodeURL, ok := service.remoteNodeURL(request.NodeID)
	if !ok {
		return siteapi.BackendInitializationJob{}, fmt.Errorf("unknown node %q", request.NodeID)
	}
	var job siteapi.BackendInitializationJob
	err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/backends/init/cancel", request, &job)
	return job, err
}

func (service *Service) initializeLocalBackend(ctx context.Context, request siteapi.BackendInitializationRequest) (siteapi.BackendInitializationJob, error) {
	if service.vllm == nil {
		return siteapi.BackendInitializationJob{}, fmt.Errorf("vllm companion is unavailable")
	}
	return service.vllm.StartInitialization(ctx, vllm.InitRequest{Profile: request.Profile})
}

func (service *Service) cancelLocalBackendInitialization(ctx context.Context, _ siteapi.BackendInitializationRequest) (siteapi.BackendInitializationJob, error) {
	if service.vllm == nil {
		return siteapi.BackendInitializationJob{}, fmt.Errorf("vllm companion is unavailable")
	}
	return service.vllm.CancelInitialization(ctx)
}

func writeBackendInitializationError(w http.ResponseWriter, err error) {
	var remoteError *cluster.RemoteError
	if errors.As(err, &remoteError) {
		writeNodeStateError(w, err)
		return
	}
	if strings.HasPrefix(err.Error(), "unknown node") {
		openai.WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if strings.Contains(err.Error(), "companion is unavailable") {
		openai.WriteError(w, http.StatusServiceUnavailable, "backend_not_initialized", err.Error())
		return
	}
	openai.WriteError(w, http.StatusBadGateway, "backend_initialization_error", err.Error())
}

func decodeNodeUnloadRequest(w http.ResponseWriter, r *http.Request) (siteapi.NodeUnloadRequest, bool) {
	defer r.Body.Close()
	var request siteapi.NodeUnloadRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return siteapi.NodeUnloadRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body must contain one JSON object")
		return siteapi.NodeUnloadRequest{}, false
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
	request.BackendID = strings.TrimSpace(request.BackendID)
	request.RuntimeID = strings.TrimSpace(request.RuntimeID)
	if request.NodeID == "" || request.BackendID == "" || request.RuntimeID == "" || request.ExpectedGeneration == 0 {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id, backend_id, runtime_id, and expected_generation are required")
		return siteapi.NodeUnloadRequest{}, false
	}
	return request, true
}

func (service *Service) nodeState(ctx context.Context, nodeID string) (siteapi.NodeState, error) {
	if nodeID == service.nodeID {
		return service.localNodeState(), nil
	}
	nodeURL, ok := service.remoteNodeURL(nodeID)
	if !ok {
		return siteapi.NodeState{}, fmt.Errorf("unknown node %q", nodeID)
	}
	var state siteapi.NodeState
	if err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/state", nil, &state); err != nil {
		return siteapi.NodeState{}, err
	}
	if state.NodeID != nodeID {
		return siteapi.NodeState{}, fmt.Errorf("node %q returned state for %q", nodeID, state.NodeID)
	}
	return state, nil
}

func (service *Service) unloadNodeRuntime(ctx context.Context, request siteapi.NodeUnloadRequest) error {
	if request.NodeID == service.nodeID {
		return service.unloadLocalRuntime(ctx, request)
	}
	nodeURL, ok := service.remoteNodeURL(request.NodeID)
	if !ok {
		return fmt.Errorf("unknown node %q", request.NodeID)
	}
	return service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/state/unload", request, nil)
}

func (service *Service) remoteNodeURL(nodeID string) (string, bool) {
	if service.registry == nil {
		return "", false
	}
	nodeURL, ok := service.registry.NodeURLsByID()[nodeID]
	return nodeURL, ok && strings.TrimSpace(nodeURL) != ""
}

func (service *Service) localNodeState() siteapi.NodeState {
	bindings := service.runtimeBindings()
	rowsByBackend := make(map[string][]siteapi.NodeStateModelRow)
	requests := make([]activeRequestSnapshot, 0)
	seenRuntimes := make(map[*backendRuntime]struct{})
	for _, binding := range bindings {
		if binding.runtime == nil {
			continue
		}
		state := binding.runtime.state
		state.mu.Lock()
		if state.modelID != "" {
			rowsByBackend[binding.backendID] = append(rowsByBackend[binding.backendID], siteapi.NodeStateModelRow{
				ModelID: state.modelID, Lane: binding.lane, RuntimeID: binding.runtime.name, Generation: state.generation,
			})
		}
		if _, seen := seenRuntimes[binding.runtime]; !seen {
			for tag, modelID := range state.leases {
				requests = append(requests, activeRequestSnapshot{tag: tag, modelID: modelID})
			}
			seenRuntimes[binding.runtime] = struct{}{}
		}
		state.mu.Unlock()
	}
	backends := make([]siteapi.NodeStateBackend, 0, len(nodeBackendDefinitions))
	for _, definition := range nodeBackendDefinitions {
		if definition.id != backendIDVLLM && !regularFile(service.backendBinaryPaths[definition.id]) {
			continue
		}
		rows := rowsByBackend[definition.id]
		if rows == nil {
			rows = []siteapi.NodeStateModelRow{}
		}
		backend := siteapi.NodeStateBackend{
			ID: definition.id, DisplayName: definition.displayName, Mode: definition.mode, LoadedModels: rows,
		}
		if definition.id == backendIDVLLM {
			backend = service.vllmNodeState(backend)
		}
		backends = append(backends, backend)
	}
	sort.Slice(requests, func(left, right int) bool { return requests[left].tag < requests[right].tag })
	activeRequests := make([]string, 0, len(requests))
	for _, request := range requests {
		if request.modelID != "" {
			activeRequests = append(activeRequests, request.modelID)
		}
	}
	return siteapi.NodeState{NodeID: service.nodeID, Backends: backends, ActiveRequests: activeRequests}
}

func (service *Service) vllmNodeState(backend siteapi.NodeStateBackend) siteapi.NodeStateBackend {
	if service.vllm == nil {
		if reason := strings.TrimSpace(service.vllmUnavailableReason); reason != "" {
			backend.LifecycleState = vllm.LifecycleFailed
			backend.Error = reason
			backend.Retryable = strings.Contains(strings.ToLower(reason), "manifest")
			if strings.Contains(strings.ToLower(reason), "unsupported") {
				backend.LifecycleState = vllm.LifecycleUnsupported
				backend.Retryable = false
			} else if strings.Contains(strings.ToLower(reason), "not found") {
				backend.LifecycleState = vllm.LifecycleCompanionMissing
				backend.Retryable = false
			}
			return backend
		}
		if regularFile(service.backendBinaryPaths[backendIDVLLM]) {
			backend.LifecycleState = vllm.LifecycleFailed
			backend.Error = "vllm companion could not be started"
			backend.Retryable = true
			return backend
		}
		backend.LifecycleState = vllm.LifecycleCompanionMissing
		backend.Error = "vllm companion is not shipped on this node"
		return backend
	}
	state := service.vllm.State(context.Background())
	backend.LifecycleState = state.LifecycleState
	backend.SelectedProfile = state.SelectedProfile
	backend.DetectedProfile = state.DetectedProfile
	backend.RuntimeVersion = state.RuntimeVersion
	backend.InitializationJobID = state.InitializationJobID
	backend.InitializationPhase = state.InitializationPhase
	backend.InitializationBytes = state.InitializationBytes
	backend.InitializationTotalBytes = state.InitializationTotalBytes
	backend.Error = state.Error
	backend.Retryable = state.Retryable
	return backend
}

func regularFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (service *Service) runtimeBindings() []runtimeBinding {
	bindings := make([]runtimeBinding, 0, 5)
	if family := service.backendFamilies[backendmode.Kobold]; family != nil {
		bindings = appendFamilyRuntimeBindings(bindings, family, backendIDKoboldCPP, backendIDKoboldCPP, backendIDKoboldCPP)
	}
	if family := service.backendFamilies[backendmode.LlamaSDCPP]; family != nil {
		bindings = appendFamilyRuntimeBindings(bindings, family, backendIDLlamaServer, backendIDSDServer, backendIDWhisper)
	}
	if family := service.backendFamilies[backendmode.VLLM]; family != nil {
		bindings = appendFamilyRuntimeBindings(bindings, family, backendIDVLLM, backendIDVLLM, backendIDVLLM)
	}
	return bindings
}

func appendFamilyRuntimeBindings(bindings []runtimeBinding, family *backendFamily, textBackendID string, imageBackendID string, transcriptionBackendID string) []runtimeBinding {
	seen := make(map[*backendRuntime]struct{}, 4)
	appendRuntime := func(runtime *backendRuntime, backendID string, lane string) {
		if runtime == nil {
			return
		}
		if _, ok := seen[runtime]; ok {
			return
		}
		seen[runtime] = struct{}{}
		bindings = append(bindings, runtimeBinding{backendID: backendID, lane: lane, runtime: runtime})
	}
	appendRuntime(family.textRuntime, textBackendID, "text")
	appendRuntime(family.embeddingsRuntime, textBackendID, "embeddings")
	appendRuntime(family.imageRuntime, imageBackendID, "image")
	appendRuntime(family.transcriptionRuntime, transcriptionBackendID, "voice")
	return bindings
}

func (service *Service) unloadLocalRuntime(ctx context.Context, request siteapi.NodeUnloadRequest) error {
	var selected *backendRuntime
	for _, binding := range service.runtimeBindings() {
		if binding.backendID == request.BackendID && binding.runtime.name == request.RuntimeID {
			selected = binding.runtime
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("backend %q and runtime %q are invalid", request.BackendID, request.RuntimeID)
	}
	service.embeddingSelection.mu.Lock()
	defer service.embeddingSelection.mu.Unlock()
	if err := service.unloadRuntimeGeneration(ctx, selected, request.ExpectedGeneration); err != nil {
		return err
	}
	if service.embeddingSelection.runtime == selected {
		service.embeddingSelection.runtime = nil
	}
	return nil
}

func (service *Service) unloadRuntimeGeneration(ctx context.Context, runtime *backendRuntime, expectedGeneration uint64) error {
	waitingSwitch := false
	state := runtime.state
	for {
		state.mu.Lock()
		if state.generation != expectedGeneration || state.modelID == "" {
			if waitingSwitch {
				state.switchWaiters--
				notifyActiveConfigLocked(state)
			}
			state.mu.Unlock()
			return errRuntimeGenerationChanged
		}
		if !waitingSwitch && state.switchWaiters > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return err
			}
			continue
		}
		if !waitingSwitch {
			state.switchWaiters++
			waitingSwitch = true
		}
		if state.switching || state.users > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				cancelConfigSwitchWaiter(state)
				return err
			}
			continue
		}
		state.switchWaiters--
		state.switching = true
		state.filename = ""
		state.modelID = ""
		state.generation++
		clearPhysicalLoadProfileLocked(state)
		clearVRAMLoadStateLocked(state)
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		err := runtime.backend.Unload(ctx)
		state.mu.Lock()
		state.switching = false
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		service.invalidateWebUIRoutes()
		return err
	}
}

func writeNodeStateError(w http.ResponseWriter, err error) {
	if errors.Is(err, errRuntimeGenerationChanged) {
		openai.WriteError(w, http.StatusConflict, "runtime_conflict", err.Error())
		return
	}
	var remoteError *cluster.RemoteError
	if errors.As(err, &remoteError) {
		status := remoteError.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		message := remoteError.Message
		if message == "" {
			message = err.Error()
		}
		openai.WriteError(w, status, remoteError.Type, message)
		return
	}
	if strings.HasPrefix(err.Error(), "unknown node") {
		openai.WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if strings.Contains(err.Error(), " are invalid") {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
}
