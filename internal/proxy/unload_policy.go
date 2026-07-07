package proxy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/unloadpolicy"
)

func (service *Service) enforceUnloadPolicy(ctx context.Context, mode string, filename string) error {
	policy, err := service.resolveUnloadPolicy(filename)
	if err != nil {
		return err
	}
	if policy == unloadpolicy.None {
		return nil
	}
	runtimes, err := service.runtimesForUnloadTarget(mode, policy)
	if err != nil {
		return err
	}
	different := make([]*backendRuntime, 0, len(runtimes))
	for _, runtime := range uniqueRuntimeList(runtimes) {
		activeFilename := currentRuntimeConfigFilename(runtime)
		if activeFilename == "" || activeFilename == filename {
			continue
		}
		different = append(different, runtime)
	}
	return service.unloadRuntimes(ctx, different)
}

func (service *Service) resolveUnloadPolicy(filename string) (string, error) {
	if strings.TrimSpace(service.configDir) == "" || strings.TrimSpace(filename) == "" {
		return unloadpolicy.None, nil
	}
	if filename != filepath.Base(filename) {
		return "", fmt.Errorf("config filename %q is invalid", filename)
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, filename))
	if err != nil {
		return "", err
	}
	return unloadpolicy.Resolve(metadata.RouterUnloadPolicy)
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
	case unloadpolicy.Text, unloadpolicy.Embeddings, unloadpolicy.Voice, unloadpolicy.Music:
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
