package proxy

import (
	"net/http"
	"testing"

	"tensors-router/internal/siteapi"
)

func TestDisabledModelUnloadStopsReportingTheModelAsLoaded(t *testing.T) {
	service, _, _ := newModelStateTestService(t)
	runtime := defaultFamilyRuntime(t, service, readinessText)
	backend := runtime.backend.(*fakeBackend)
	state := runtime.state
	state.mu.Lock()
	state.filename = "model-a.kcpps"
	state.modelID = "model-a"
	state.generation = 1
	state.mu.Unlock()

	disabled := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	waitForUnloadCount(t, backend, 1)

	state.mu.Lock()
	modelID, generation := state.modelID, state.generation
	state.mu.Unlock()
	if modelID != "" {
		t.Fatalf("unloaded runtime still reports model %q as loaded", modelID)
	}
	if generation == 1 {
		t.Fatal("unload kept the loaded generation, so a stale node unload would still match it")
	}
}
