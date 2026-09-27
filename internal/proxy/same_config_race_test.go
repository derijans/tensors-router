package proxy

import (
	"testing"

	"tensors-router/internal/catalog"
)

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
