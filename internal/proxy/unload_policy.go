package proxy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/unloadpolicy"
)

func (service *Service) enforceUnloadPolicy(ctx context.Context, mode string, filename string, readiness backendReadiness) error {
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return err
	}
	policy, err := service.resolveUnloadPolicy(filename)
	if err != nil {
		return err
	}
	if policy.IsNone() {
		return nil
	}
	runtimes, err := service.runtimesForUnloadPolicy(resolvedMode, policy)
	if err != nil {
		return err
	}
	different := make([]*backendRuntime, 0, len(runtimes))
	profile := service.chatTemplateProfileForConfig(filename)
	for _, runtime := range uniqueRuntimeList(runtimes) {
		if service.separatePoolOwns(runtime) {
			continue
		}
		if currentRuntimeConfigFilename(runtime) == "" || activeRuntimeSupportsConfig(runtime, filename, profile) {
			continue
		}
		different = append(different, runtime)
	}
	return service.unloadRuntimes(ctx, different)
}

// separatePoolOwns is checked to skip pool runtimes here: they answer only to
// their own triggers, never another config's unload policy.
func (service *Service) separatePoolOwns(runtime *backendRuntime) bool {
	if service.separatePool == nil || runtime == nil {
		return false
	}
	for _, entry := range service.separatePool.snapshot() {
		if entry.runtime == runtime {
			return true
		}
	}
	return false
}

func (service *Service) resolveUnloadPolicy(filename string) (unloadpolicy.Selection, error) {
	if strings.TrimSpace(service.configDir) == "" || strings.TrimSpace(filename) == "" {
		return unloadpolicy.Selection{unloadpolicy.None}, nil
	}
	if filename != filepath.Base(filename) {
		return nil, fmt.Errorf("config filename %q is invalid", filename)
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, filename))
	if err != nil {
		return nil, err
	}
	return unloadpolicy.ResolveSelection(metadata.RouterUnloadPolicy)
}

// runtimesForUnloadPolicy is the union of the shared runtimes every trigger names.
func (service *Service) runtimesForUnloadPolicy(mode string, policy unloadpolicy.Selection) ([]*backendRuntime, error) {
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return nil, err
	}
	family := service.backendFamilies[resolvedMode]
	if family == nil {
		return nil, fmt.Errorf("backend mode %q is not configured", resolvedMode)
	}
	runtimes := make([]*backendRuntime, 0, 4)
	for _, trigger := range policy {
		switch {
		case trigger == unloadpolicy.None:
			continue
		case unloadpolicy.ValidLane(trigger) || trigger == unloadpolicy.All:
			laneRuntimes, err := service.runtimesForUnloadTarget(resolvedMode, trigger)
			if err != nil {
				return nil, err
			}
			runtimes = append(runtimes, laneRuntimes...)
		default:
			if familyMode, ok := unloadpolicy.FamilyTarget(trigger); ok {
				if familyMode == resolvedMode {
					runtimes = append(runtimes, uniqueBackendRuntimes(family)...)
				}
				continue
			}
			if configID, ok := unloadpolicy.ConfigTarget(trigger); ok {
				runtimes = append(runtimes, service.sharedRuntimesHoldingConfig(family, configID)...)
			}
		}
	}
	return runtimes, nil
}

func (service *Service) sharedRuntimesHoldingConfig(family *backendFamily, configID string) []*backendRuntime {
	holders := make([]*backendRuntime, 0, 1)
	for _, runtime := range uniqueBackendRuntimes(family) {
		filename := currentRuntimeConfigFilename(runtime)
		if filename == "" {
			continue
		}
		base := filepath.Base(filename)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if base == configID || stem == configID {
			holders = append(holders, runtime)
		}
	}
	return holders
}

func (service *Service) runtimesForUnloadTarget(mode string, target string) ([]*backendRuntime, error) {
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return nil, err
	}
	family := service.backendFamilies[resolvedMode]
	if family == nil {
		return nil, fmt.Errorf("backend mode %q is not configured", resolvedMode)
	}
	target, err = unloadpolicy.ResolveTarget(target)
	if err != nil {
		return nil, err
	}
	switch target {
	case unloadpolicy.All:
		return uniqueBackendRuntimes(family), nil
	case unloadpolicy.Image:
		return []*backendRuntime{family.imageRuntime}, nil
	case unloadpolicy.Embeddings:
		return []*backendRuntime{family.embeddingsRuntime}, nil
	case unloadpolicy.Voice:
		return []*backendRuntime{family.textRuntime, family.transcriptionRuntime}, nil
	case unloadpolicy.Text, unloadpolicy.Music:
		return []*backendRuntime{family.textRuntime}, nil
	default:
		return nil, fmt.Errorf("unload target %q is invalid", target)
	}
}

func (service *Service) unloadRuntimes(ctx context.Context, runtimes []*backendRuntime) error {
	runtimes = uniqueRuntimeList(runtimes)
	if len(runtimes) == 0 {
		return nil
	}
	if len(runtimes) == 1 {
		return service.unloadRuntime(ctx, runtimes[0])
	}
	errors := make(chan error, len(runtimes))
	for _, runtime := range runtimes {
		runtime := runtime
		go func() {
			errors <- service.unloadRuntime(ctx, runtime)
		}()
	}
	var firstErr error
	for range runtimes {
		if err := <-errors; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func uniqueRuntimeList(runtimes []*backendRuntime) []*backendRuntime {
	seen := map[*backendRuntime]struct{}{}
	unique := make([]*backendRuntime, 0, len(runtimes))
	for _, runtime := range runtimes {
		if runtime == nil {
			continue
		}
		if _, ok := seen[runtime]; ok {
			continue
		}
		seen[runtime] = struct{}{}
		unique = append(unique, runtime)
	}
	return unique
}
