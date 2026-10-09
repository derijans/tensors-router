package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// backendProcessAlive reports whether the managed backend process is still running. A
// backend that cannot report its exit state is treated as not alive, so the caller keeps
// its existing restart behaviour.
func (service *Service) backendProcessAlive(runtime *backendRuntime) bool {
	reporter, ok := runtime.backend.(backendExitReporter)
	if !ok {
		return false
	}
	return reporter.BackendExitError() == nil
}

// waitForBackendProcess waits for a backend that is restarting itself to start answering
// again. It gives up as soon as the process exits, so a genuinely dead backend still
// falls through to a restart instead of being waited on.
func (service *Service) waitForBackendProcess(runtime *backendRuntime, ctx context.Context) error {
	deadline := time.Now().Add(backendSelfRestartWait)
	for {
		if runtime.backend.Healthy(ctx) {
			return nil
		}
		if !service.backendProcessAlive(runtime) {
			return fmt.Errorf("backend process exited while restarting")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("backend did not come back within %s", backendSelfRestartWait)
		}
		if err := waitForRetry(ctx, service.backendRetryDelay); err != nil {
			return err
		}
	}
}

func (service *Service) reloadModelConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string) error {
	service.logger.Printf("config switch reload attempt model=%q config=%q", modelID, configFilename)
	if reloadErr := runtime.backend.ReloadConfig(ctx, configFilename); reloadErr != nil {
		service.logger.Printf("config switch reload failed model=%q config=%q error=%v", modelID, configFilename, reloadErr)
		if runtime.backend.Healthy(ctx) {
			return reloadErr
		}

		// An unreachable backend is not necessarily a dead one. KoboldCpp restarts itself
		// to apply an admin reload, so its port is closed for several seconds afterwards —
		// and a reload arriving in that window fails with "connection refused" while the
		// process is perfectly fine. Restarting it here kills the load already in progress
		// and starts another, which is how one switch turned into two process starts and,
		// when it overlapped again, a load that never finished at all.
		//
		// The managed process tells us which case this is: if it has not exited, wait for
		// it to finish coming back and reload again rather than restarting it.
		if service.backendProcessAlive(runtime) {
			service.logger.Printf("backend restarting itself after config switch; waiting model=%q config=%q", modelID, configFilename)
			if waitErr := service.waitForBackendProcess(runtime, ctx); waitErr == nil {
				service.logger.Printf("config switch reload retry after backend restart model=%q config=%q", modelID, configFilename)
				if retryErr := runtime.backend.ReloadConfig(ctx, configFilename); retryErr == nil {
					service.logger.Printf("config switch reload succeeded model=%q config=%q", modelID, configFilename)
					return nil
				}
			}
		}

		service.logger.Printf("backend unhealthy after config switch failure model=%q config=%q", modelID, configFilename)
		restartContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		service.logger.Printf("backend restart attempt model=%q config=%q lane=%s", modelID, configFilename, runtime.name)
		if restartErr := runtime.backend.Restart(restartContext); restartErr != nil {
			service.logger.Printf("backend restart failed model=%q config=%q lane=%s error=%v", modelID, configFilename, runtime.name, restartErr)
			return fmt.Errorf("reload failed: %v; restart failed: %w", reloadErr, restartErr)
		}

		service.logger.Printf("config switch reload retry model=%q config=%q", modelID, configFilename)
		if retryErr := runtime.backend.ReloadConfig(ctx, configFilename); retryErr != nil {
			return fmt.Errorf("reload failed after restart: %w", retryErr)
		}
	}

	service.logger.Printf("config switch reload succeeded model=%q config=%q", modelID, configFilename)
	return nil
}

func (service *Service) waitForBackendEndpoint(runtime *backendRuntime, ctx context.Context, readiness backendReadiness, modelID string, configFilename string) error {
	// No watch was started before the load, so anything the backend printed while it was
	// loading is already gone. Start one now and rely on the bounded gate.
	watch := service.watchBackendReadiness(runtime, readiness)
	defer watch.close()
	return service.waitForBackendEndpointWatching(runtime, ctx, readiness, modelID, configFilename, watch)
}

func (service *Service) backendReadinessTimeout() time.Duration {
	if service.backendReadinessWait > 0 {
		return service.backendReadinessWait
	}
	return defaultBackendReadinessWait
}

// errBackendServingNoModel marks the one readiness failure worth repeating the reload
// for: the backend is up and answering, but reports it is holding no model at all. That
// is the signature of a reload that was accepted and then lost, which only another reload
// fixes. Every other readiness failure — unreachable, exited, capability disabled — is
// reported as-is so recovery stays bounded.
var errBackendServingNoModel = errors.New("backend is serving no model")

func probeAnswersWithoutModel(readiness backendReadiness, status int, body string) bool {
	return probeReportsNoModel(status, body) || readiness == readinessImage && probeListsNoImageModel(status, body)
}

func probeListsNoImageModel(status int, body string) bool {
	if status < 200 || status >= 300 {
		return false
	}
	var models []map[string]any
	return json.Unmarshal([]byte(strings.TrimSpace(body)), &models) == nil && len(models) == 0
}

func probeReportsNoModel(status int, body string) bool {
	if status < 200 || status >= 300 {
		return false
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &parsed); err != nil {
		return false
	}
	if len(parsed.Data) == 0 {
		return false
	}
	for _, model := range parsed.Data {
		if !strings.EqualFold(strings.TrimSpace(model.ID), "inactive") {
			return false
		}
	}
	return true
}

func backendEndpointReady(readiness backendReadiness, backendMode string, status int, body string) bool {
	if status < 200 || status >= 300 {
		return false
	}
	if readiness == readinessImage {
		return backendImageEndpointReady(body)
	}
	if backendMode == BackendModeVLLM || readiness == readinessTranscription && backendMode == BackendModeLlamaSDCPP {
		return true
	}
	if readiness == readinessEmbeddings && backendMode == BackendModeKobold {
		return backendCapabilityReady(body, "embeddings")
	}
	if capability := readiness.capability(); capability != "" {
		return backendCapabilityReady(body, capability)
	}
	return backendTextEndpointReady(body)
}

func backendImageEndpointReady(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	var parsedList []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsedList); err == nil {
		return imageReadinessListHasModel(parsedList)
	}
	var parsedObject struct {
		Model     string           `json:"model"`
		ID        string           `json:"id"`
		Title     string           `json:"title"`
		ModelName string           `json:"model_name"`
		Data      []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(trimmed), &parsedObject); err != nil {
		return true
	}
	if parsedObject.Data != nil {
		return imageReadinessListHasModel(parsedObject.Data)
	}
	return imageReadinessModelNameReady(parsedObject.Model) ||
		imageReadinessModelNameReady(parsedObject.ID) ||
		imageReadinessModelNameReady(parsedObject.Title) ||
		imageReadinessModelNameReady(parsedObject.ModelName)
}

func imageReadinessListHasModel(models []map[string]any) bool {
	for _, model := range models {
		for _, key := range []string{"model_name", "title", "model", "id"} {
			if imageReadinessValueReady(model[key]) {
				return true
			}
		}
	}
	return false
}

func imageReadinessValueReady(value any) bool {
	text, ok := value.(string)
	return ok && imageReadinessModelNameReady(text)
}

func imageReadinessModelNameReady(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && !strings.EqualFold(trimmed, "inactive")
}

func backendTextEndpointReady(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	var parsed struct {
		Model string `json:"model"`
		ID    string `json:"id"`
		Data  []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(parsed.Model), "inactive") || strings.EqualFold(strings.TrimSpace(parsed.ID), "inactive") {
		return false
	}
	if parsed.Data == nil {
		return true
	}
	hasID := false
	for _, model := range parsed.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		hasID = true
		if !strings.EqualFold(id, "inactive") {
			return true
		}
	}
	return !hasID
}
