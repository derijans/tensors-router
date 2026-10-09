package proxy

import (
	"context"

	"tensors-router/internal/catalog"
)

type configAcquisition struct {
	runtime        *backendRuntime
	modelID        string
	configFilename string
	readiness      backendReadiness
	profile        catalog.ChatTemplateProfile
	borrowed       bool
}

func (service *Service) acquireModelConfigWithOptions(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, options modelConfigAcquireOptions) (func(), bool, error) {
	request := configAcquisition{
		runtime:        runtime,
		modelID:        modelID,
		configFilename: configFilename,
		readiness:      readiness,
		profile:        service.chatTemplateProfileForConfig(configFilename),
		borrowed:       contextIsBorrowed(ctx),
	}
	state := runtime.state
	satisfiedBy := func(current *activeConfigState) bool {
		return !options.forceReload && activeConfigMatchesAcquireOptions(current, configFilename, request.profile, options)
	}
	state.mu.Lock()
	ticket := state.takeTicketLocked()
	queued := false
	for {
		if satisfiedBy(state) && !state.switching && state.admitsLeaseLocked(ticket) {
			return service.reuseActiveConfigLocked(state, ticket, request), false, nil
		}
		if !queued {
			state.joinQueueLocked(ticket, satisfiedBy)
			queued = true
		}
		if satisfiedBy(state) || state.switching || state.users > 0 || !state.maySwitchLocked(ticket) {
			if err := waitForConfigTurnLocked(ctx, state, ticket); err != nil {
				return nil, false, err
			}
			continue
		}
		state.leaveQueueLocked(ticket)
		beginRuntimeSwitchLocked(state, request.borrowed)
		state.pendingFilename = configFilename
		state.pendingProfile = request.profile
		state.mu.Unlock()
		return service.switchRuntimeConfig(ctx, request)
	}
}

func waitForConfigTurnLocked(ctx context.Context, state *activeConfigState, ticket uint64) error {
	changed := state.changed
	state.mu.Unlock()
	if err := waitForActiveConfigChange(ctx, changed); err != nil {
		leaveSwitchQueue(state, ticket)
		return err
	}
	state.mu.Lock()
	return nil
}

func (service *Service) reuseActiveConfigLocked(state *activeConfigState, ticket uint64, request configAcquisition) func() {
	state.leaveQueueLocked(ticket)
	logicalConfigChanged := state.filename != request.configFilename
	logicalModelChanged := state.modelID != request.modelID
	state.filename = request.configFilename
	state.modelID = request.modelID
	if logicalConfigChanged || logicalModelChanged {
		state.generation++
	}
	release := service.addRuntimeLeaseLocked(state, request.modelID, request.borrowed)
	physicalAttemptID := state.physicalAttemptID
	state.mu.Unlock()
	service.recordLoadReuse(physicalAttemptID)
	if logicalConfigChanged {
		service.onRuntimeChanged()
	}
	return release
}

func (service *Service) switchRuntimeConfig(ctx context.Context, request configAcquisition) (func(), bool, error) {
	runtime, state := request.runtime, request.runtime.state
	capture, err := service.beginPhysicalLoadCapture(ctx, runtime, request.configFilename, request.readiness)
	if err != nil {
		service.abandonRuntimeSwitch(state)
		return nil, false, err
	}
	loadMeasurement := service.beginModelLoad(ctx)
	err = service.loadModelConfig(runtime, ctx, request.modelID, request.configFilename, request.readiness)
	service.finishModelLoad(ctx, loadMeasurement)
	service.finishPhysicalLoadCapture(capture, err)
	if err != nil {
		service.abandonRuntimeSwitch(state)
		return nil, false, err
	}

	state.mu.Lock()
	endRuntimeSwitchLocked(state)
	clearPendingConfigLocked(state)
	state.filename = request.configFilename
	state.modelID = request.modelID
	state.generation++
	state.physicalAttemptID = ""
	if capture != nil {
		state.physicalAttemptID = capture.attempt.ID
	}
	applyPhysicalLoadProfileLocked(state, request.configFilename, request.profile, request.readiness)
	applyLoadMeasurementLocked(state, loadMeasurement)
	state.openLeaseWindowLocked()
	release := service.addRuntimeLeaseLocked(state, request.modelID, request.borrowed)
	notifyActiveConfigLocked(state)
	state.mu.Unlock()
	service.analytics.recordLoad(request.modelID, request.configFilename, request.readiness, runtime.mode, loadMeasurement.analytics)
	service.onRuntimeChanged()
	return release, true, nil
}

func (service *Service) abandonRuntimeSwitch(state *activeConfigState) {
	state.mu.Lock()
	endRuntimeSwitchLocked(state)
	clearPendingConfigLocked(state)
	state.filename = ""
	state.modelID = ""
	state.physicalAttemptID = ""
	clearPhysicalLoadProfileLocked(state)
	clearLoadMeasurementLocked(state)
	notifyActiveConfigLocked(state)
	state.mu.Unlock()
	service.onRuntimeChanged()
}

func clearPendingConfigLocked(state *activeConfigState) {
	state.pendingFilename = ""
	state.pendingProfile = catalog.ChatTemplateProfile{}
}
