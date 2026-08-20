package proxy

import (
	"testing"

	"tensors-router/internal/catalog"
)

// While a load is in flight the runtime reports the config it is loading, not the one it
// previously held. Answering "no, this runtime does not support that config" for the very
// config being loaded is what let a concurrent caller schedule an unload of a load it was
// itself waiting for — and every unload SIGINTs the backend, leaving KoboldCpp serving no
// model while the router polls it for "inactive".
func TestRuntimeSupportsTheConfigItIsCurrentlyLoading(t *testing.T) {
	runtime := &backendRuntime{state: newActiveConfigState(), mode: BackendModeKobold}
	profile := catalog.ChatTemplateProfile{}

	runtime.state.switching = true
	runtime.state.filename = "previous.kcpps"
	runtime.state.pendingFilename = "wanted.kcpps"

	if !activeRuntimeSupportsConfig(runtime, "wanted.kcpps", profile) {
		t.Fatal("a runtime loading wanted.kcpps must report that it supports wanted.kcpps")
	}
	if activeRuntimeSupportsConfig(runtime, "unrelated.kcpps", profile) {
		t.Fatal("a runtime loading wanted.kcpps must not claim to support an unrelated config")
	}
}

// Once the load finishes the pending target is cleared, so support is decided by what the
// runtime actually holds.
func TestPendingConfigIsClearedAfterSwitch(t *testing.T) {
	runtime := &backendRuntime{state: newActiveConfigState(), mode: BackendModeKobold}
	profile := catalog.ChatTemplateProfile{}

	runtime.state.switching = false
	runtime.state.filename = "wanted.kcpps"
	runtime.state.pendingFilename = ""

	if !activeRuntimeSupportsConfig(runtime, "wanted.kcpps", profile) {
		t.Fatal("a settled runtime must support the config it holds")
	}
	if activeRuntimeSupportsConfig(runtime, "previous.kcpps", profile) {
		t.Fatal("a settled runtime must not support a config it no longer holds")
	}
}

// A runtime that is switching with no recorded target cannot satisfy anyone; it must not
// fall through to matching on the stale filename it is in the middle of replacing.
func TestSwitchingRuntimeWithoutPendingTargetSupportsNothing(t *testing.T) {
	runtime := &backendRuntime{state: newActiveConfigState(), mode: BackendModeKobold}
	profile := catalog.ChatTemplateProfile{}

	runtime.state.switching = true
	runtime.state.filename = "previous.kcpps"
	runtime.state.pendingFilename = ""

	if activeRuntimeSupportsConfig(runtime, "previous.kcpps", profile) {
		t.Fatal("a runtime mid-switch must not still claim the config it is replacing")
	}
}
