package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"tensors-router/internal/openai"
)

type vllmAdminRoute struct {
	method       string
	backendPath  string
	readiness    backendReadiness
	requiresLoRA bool
	requiresEEP  bool
}

var vllmAdminRoutes = map[string]vllmAdminRoute{
	"/router/v1/vllm/health":         {method: http.MethodGet, backendPath: "/health", readiness: readinessText},
	"/router/v1/vllm/version":        {method: http.MethodGet, backendPath: "/version", readiness: readinessText},
	"/router/v1/vllm/load":           {method: http.MethodGet, backendPath: "/load", readiness: readinessText},
	"/router/v1/vllm/metrics":        {method: http.MethodGet, backendPath: "/metrics", readiness: readinessText},
	"/router/v1/vllm/tokenizer-info": {method: http.MethodGet, backendPath: "/tokenizer_info", readiness: readinessText},
	"/router/v1/vllm/lora/load":      {method: http.MethodPost, backendPath: "/v1/load_lora_adapter", readiness: readinessText, requiresLoRA: true},
	"/router/v1/vllm/lora/unload":    {method: http.MethodPost, backendPath: "/v1/unload_lora_adapter", readiness: readinessText, requiresLoRA: true},
	"/router/v1/vllm/eep/scale":      {method: http.MethodPost, backendPath: "/scale_elastic_ep", readiness: readinessText, requiresEEP: true},
	"/router/v1/vllm/eep/status":     {method: http.MethodGet, backendPath: "/is_scaling_elastic_ep", readiness: readinessText, requiresEEP: true},
}

func (service *Service) handleVLLMAdmin(w http.ResponseWriter, r *http.Request) {
	route, found := vllmAdminRoutes[r.URL.Path]
	if !found || r.Method != route.method {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if route.requiresLoRA && !service.vllmDynamicLoRAEnabled {
		openai.WriteError(w, http.StatusForbidden, "feature_disabled", "dynamic LoRA is disabled")
		return
	}
	if route.requiresEEP && !service.vllmEEPEnabled {
		openai.WriteError(w, http.StatusForbidden, "feature_disabled", "Elastic Expert Parallelism is disabled")
		return
	}
	readiness, err := vllmAdminReadiness(r.URL.Query().Get("runtime"), route.readiness)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	runtime, err := service.runtimeForBackendMode(BackendModeVLLM, readiness)
	if err != nil || runtime == nil {
		openai.WriteError(w, http.StatusServiceUnavailable, "backend_not_initialized", "vLLM runtime is not configured")
		return
	}
	release, ok := acquireLoadedRuntimeLease(service, runtime)
	if !ok {
		openai.WriteError(w, http.StatusServiceUnavailable, "backend_not_initialized", "vLLM runtime is not loaded")
		return
	}
	defer release()
	body, ok := service.readVLLMRequestBody(w, r)
	if !ok {
		return
	}
	forwarded := r.Clone(r.Context())
	forwarded.URL.Path = route.backendPath
	forwarded.URL.RawPath = ""
	forwarded.URL.RawQuery = ""
	response, err := service.forward(runtime, r.Context(), forwarded, body)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	if err := service.writeTransportResponse(w, response, "", false); err != nil {
		service.logger.Printf("vLLM admin response failed path=%s error=%v", r.URL.Path, err)
	}
}

func vllmAdminReadiness(runtimeName string, fallback backendReadiness) (backendReadiness, error) {
	switch strings.ToLower(strings.TrimSpace(runtimeName)) {
	case "", "generation":
		return fallback, nil
	case "pooling":
		return readinessEmbeddings, nil
	case "speech":
		return readinessTranscription, nil
	default:
		return readinessText, fmt.Errorf("runtime must be generation, pooling, or speech")
	}
}

func acquireLoadedRuntimeLease(service *Service, runtime *backendRuntime) (func(), bool) {
	state := runtime.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.switching || strings.TrimSpace(state.filename) == "" {
		return nil, false
	}
	state.users++
	leaseTag := service.nextRuntimeLease.Add(1)
	state.leases[leaseTag] = state.modelID
	return releaseActiveConfigLeaseOnce(state, leaseTag), true
}
