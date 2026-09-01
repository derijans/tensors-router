package proxy

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/unloadpolicy"
)

// separateBackendFactory builds a fresh dynamic-port backend for one pooled config.
// lane "embeddings" selects the --nomodel embeddings-role manager.
type separateBackendFactory func(name string, lane string) (Backend, error)

type separateManagedBackend interface {
	Start(context.Context) error
}

type separateStoppableBackend interface {
	Stop(context.Context) error
}

type separateEndpointReleaser interface {
	ReleaseEndpoint()
}

const defaultSeparateRuntimeLimit = 5

// separateRuntimeEntry is one co-resident backend process holding a single config
// on its own activeConfigState, untouched by the shared runtime's churn.
type separateRuntimeEntry struct {
	localID   string
	filename  string
	mode      string
	lane      string
	readiness backendReadiness
	runtime   *backendRuntime
	triggers  unloadpolicy.Selection
	lastUsed  time.Time
}

// separateRuntimePool caps co-resident separate processes, evicting strict LRU.
type separateRuntimePool struct {
	mu      sync.Mutex
	limit   int
	entries map[string]*separateRuntimeEntry
}

func newSeparateRuntimePool(limit int) *separateRuntimePool {
	if limit < 1 {
		limit = defaultSeparateRuntimeLimit
	}
	return &separateRuntimePool{limit: limit, entries: map[string]*separateRuntimeEntry{}}
}

func separateLaneLabel(readiness backendReadiness) string {
	switch readiness {
	case readinessEmbeddings:
		return "embeddings"
	case readinessImage:
		return "image"
	case readinessSpeech, readinessTranscription:
		return "voice"
	case readinessMusic:
		return "music"
	default:
		return "text"
	}
}

// configRunsSeparate takes the per-node store override when one exists (it isolates
// the whole config, on any lane); otherwise the legacy run_embed_separate flag
// isolates only the embeddings lane.
func (service *Service) configRunsSeparate(ctx context.Context, modelID string, readiness backendReadiness, metadata catalog.RuntimeConfig) (bool, unloadpolicy.Selection, error) {
	if service.modelStateStore != nil && strings.TrimSpace(modelID) != "" {
		settings, present, err := service.modelStateStore.SeparateRuntime(ctx, modelID)
		if err != nil {
			return false, nil, err
		}
		if present {
			resolved, err := unloadpolicy.ResolveSelection(unloadpolicy.Selection(settings.UnloadTriggers))
			if err != nil {
				return false, nil, err
			}
			return settings.RunSeparate, resolved, nil
		}
	}
	resolved, err := unloadpolicy.ResolveSelection(metadata.RouterUnloadPolicy)
	if err != nil {
		return false, nil, err
	}
	return readiness == readinessEmbeddings && metadata.RunEmbedSeparate, resolved, nil
}

// tryAcquireSeparateConfig routes a separate-marked config to its pool entry.
// handled=false means the caller must take the shared path instead.
func (service *Service) tryAcquireSeparateConfig(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, options modelConfigAcquireOptions) (handled bool, runtime *backendRuntime, release func(), loadedFresh bool, err error) {
	if service.separatePool == nil || strings.TrimSpace(configFilename) == "" || service.configDir == "" {
		return false, nil, nil, false, nil
	}
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return false, nil, nil, false, err
	}
	family := service.backendFamilies[resolvedMode]
	if family == nil || family.separateBackend == nil {
		return false, nil, nil, false, nil
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, configFilename))
	if err != nil {
		return false, nil, nil, false, err
	}
	runSeparate, triggers, err := service.configRunsSeparate(ctx, modelID, readiness, metadata)
	if err != nil {
		return false, nil, nil, false, err
	}
	if !runSeparate {
		return false, nil, nil, false, nil
	}
	runtime, release, loadedFresh, err = service.acquireSeparateConfig(family, ctx, modelID, configFilename, readiness, triggers, options)
	return true, runtime, release, loadedFresh, err
}

func (service *Service) acquireSeparateConfig(family *backendFamily, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, triggers unloadpolicy.Selection, options modelConfigAcquireOptions) (*backendRuntime, func(), bool, error) {
	entry, evicted, err := service.separatePool.reserveEntry(family, modelID, configFilename, readiness, triggers)
	if err != nil {
		return nil, nil, false, err
	}
	for _, victim := range evicted {
		service.teardownSeparateEntry(ctx, victim)
	}
	if starter, ok := entry.runtime.backend.(separateManagedBackend); ok {
		if err := starter.Start(ctx); err != nil {
			return entry.runtime, nil, false, err
		}
	}
	release, loadedFresh, err := service.acquireModelConfigWithOptions(entry.runtime, ctx, modelID, configFilename, readiness, options)
	if err != nil {
		return entry.runtime, nil, false, err
	}
	service.separatePool.touch(configFilename)
	service.applySeparateRuntimeTriggers(ctx, entry.mode, configFilename, modelID)
	return entry.runtime, release, loadedFresh, nil
}

// reserveEntry returns the config's entry, plus any residents it had to evict to
// stay within the cap for a freshly created one.
func (pool *separateRuntimePool) reserveEntry(family *backendFamily, modelID string, configFilename string, readiness backendReadiness, triggers unloadpolicy.Selection) (*separateRuntimeEntry, []*separateRuntimeEntry, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if entry, ok := pool.entries[configFilename]; ok {
		entry.triggers = triggers
		entry.lastUsed = time.Now()
		return entry, nil, nil
	}
	var evicted []*separateRuntimeEntry
	for len(pool.entries) >= pool.limit {
		victim := pool.leastRecentlyUsedLocked()
		if victim == nil {
			break
		}
		delete(pool.entries, victim.filename)
		evicted = append(evicted, victim)
	}
	lane := separateLaneLabel(readiness)
	name := family.mode + "-separate-" + separateRuntimeSlug(modelID, configFilename)
	backend, err := family.separateBackend(name, lane)
	if err != nil {
		return nil, evicted, err
	}
	entry := &separateRuntimeEntry{
		localID:   modelID,
		filename:  configFilename,
		mode:      family.mode,
		lane:      lane,
		readiness: readiness,
		runtime:   &backendRuntime{backend: backend, state: newActiveConfigState(), mode: family.mode, name: name},
		triggers:  triggers,
		lastUsed:  time.Now(),
	}
	pool.entries[configFilename] = entry
	return entry, evicted, nil
}

func (pool *separateRuntimePool) leastRecentlyUsedLocked() *separateRuntimeEntry {
	var oldest *separateRuntimeEntry
	for _, entry := range pool.entries {
		if oldest == nil || entry.lastUsed.Before(oldest.lastUsed) {
			oldest = entry
		}
	}
	return oldest
}

func (pool *separateRuntimePool) touch(configFilename string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if entry, ok := pool.entries[configFilename]; ok {
		entry.lastUsed = time.Now()
	}
}

func (pool *separateRuntimePool) snapshot() []*separateRuntimeEntry {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	entries := make([]*separateRuntimeEntry, 0, len(pool.entries))
	for _, entry := range pool.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].name() < entries[right].name() })
	return entries
}

func (entry *separateRuntimeEntry) name() string {
	if entry.runtime == nil {
		return entry.filename
	}
	return entry.runtime.name
}

func (pool *separateRuntimePool) remove(configFilename string) *separateRuntimeEntry {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	entry := pool.entries[configFilename]
	delete(pool.entries, configFilename)
	return entry
}

// applySeparateRuntimeTriggers evicts pool entries whose triggers match the config
// that just finished loading anywhere else.
func (service *Service) applySeparateRuntimeTriggers(ctx context.Context, loadingMode string, loadingFilename string, loadingModelID string) {
	if service.separatePool == nil {
		return
	}
	lanes := service.configLaneSet(loadingFilename)
	stems := configIdentitySet(loadingModelID, loadingFilename)
	var victims []*separateRuntimeEntry
	for _, entry := range service.separatePool.snapshot() {
		if entry.filename == loadingFilename {
			continue
		}
		if separateEntryTriggeredBy(entry.triggers, loadingMode, lanes, stems) {
			victims = append(victims, entry)
		}
	}
	for _, victim := range victims {
		if removed := service.separatePool.remove(victim.filename); removed != nil {
			service.teardownSeparateEntry(ctx, removed)
		}
	}
}

func separateEntryTriggeredBy(triggers unloadpolicy.Selection, loadingMode string, lanes map[string]struct{}, identities map[string]struct{}) bool {
	for _, trigger := range triggers {
		switch {
		case trigger == unloadpolicy.None:
			continue
		case trigger == unloadpolicy.All:
			return true
		case unloadpolicy.ValidLane(trigger):
			if _, ok := lanes[unloadpolicy.Normalize(trigger)]; ok {
				return true
			}
		default:
			if mode, ok := unloadpolicy.FamilyTarget(trigger); ok && mode == loadingMode {
				return true
			}
			if id, ok := unloadpolicy.ConfigTarget(trigger); ok {
				if _, matched := identities[id]; matched {
					return true
				}
			}
		}
	}
	return false
}

func configIdentitySet(modelID string, filename string) map[string]struct{} {
	identities := map[string]struct{}{}
	if trimmed := strings.TrimSpace(modelID); trimmed != "" {
		identities[trimmed] = struct{}{}
	}
	base := filepath.Base(strings.TrimSpace(filename))
	if base != "" && base != "." {
		identities[base] = struct{}{}
		identities[strings.TrimSuffix(base, filepath.Ext(base))] = struct{}{}
	}
	return identities
}

func (service *Service) configLaneSet(filename string) map[string]struct{} {
	lanes := map[string]struct{}{}
	if service.catalog == nil || strings.TrimSpace(filename) == "" {
		return lanes
	}
	models, err := service.catalog.List()
	if err != nil {
		return lanes
	}
	for _, model := range models {
		if model.Filename != filename {
			continue
		}
		separateEmbeddings := model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate
		if model.HasLLM || model.HasMultimodal || model.HasEmbeddings && !separateEmbeddings {
			lanes[unloadpolicy.Text] = struct{}{}
		}
		if separateEmbeddings {
			lanes[unloadpolicy.Embeddings] = struct{}{}
		}
		if model.HasImage {
			lanes[unloadpolicy.Image] = struct{}{}
		}
		if model.HasVoice {
			lanes[unloadpolicy.Voice] = struct{}{}
		}
		if model.HasMusic {
			lanes[unloadpolicy.Music] = struct{}{}
		}
	}
	return lanes
}

// teardownSeparateEntry stops the process and frees its port. The switch lock is
// taken first so a load in flight is not severed mid-way.
func (service *Service) teardownSeparateEntry(ctx context.Context, entry *separateRuntimeEntry) {
	if entry == nil || entry.runtime == nil {
		return
	}
	unlock, err := lockRuntimeForBackendStop(ctx, entry.runtime)
	if err != nil {
		service.logger.Printf("separate runtime teardown could not lock runtime=%s error=%v", entry.runtime.name, err)
		return
	}
	defer unlock()
	if stopper, ok := entry.runtime.backend.(separateStoppableBackend); ok {
		if err := stopper.Stop(ctx); err != nil {
			service.logger.Printf("separate runtime stop failed runtime=%s error=%v", entry.runtime.name, err)
		}
	} else if err := entry.runtime.backend.Unload(ctx); err != nil {
		service.logger.Printf("separate runtime unload failed runtime=%s error=%v", entry.runtime.name, err)
	}
	if releaser, ok := entry.runtime.backend.(separateEndpointReleaser); ok {
		releaser.ReleaseEndpoint()
	}
	service.invalidateWebUIRoutes()
}

// unloadSeparateLane unloads pool entries on a lane but keeps them, so a later
// request reloads in place.
func (service *Service) unloadSeparateLane(ctx context.Context, lane string) error {
	if service.separatePool == nil {
		return nil
	}
	var firstErr error
	for _, entry := range service.separatePool.snapshot() {
		if lane != "" && lane != unloadpolicy.All && entry.lane != lane {
			continue
		}
		if err := service.unloadRuntime(ctx, entry.runtime); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (service *Service) separateRuntimeBindings(bindings []runtimeBinding) []runtimeBinding {
	if service.separatePool == nil {
		return bindings
	}
	for _, entry := range service.separatePool.snapshot() {
		backendID := separateBackendIDForMode(entry.mode)
		if backendID == "" {
			continue
		}
		bindings = append(bindings, runtimeBinding{backendID: backendID, lane: entry.lane, runtime: entry.runtime})
	}
	return bindings
}

func separateBackendIDForMode(mode string) string {
	switch mode {
	case BackendModeKobold:
		return backendIDKoboldCPP
	case BackendModeLlamaSDCPP:
		return backendIDLlamaServer
	default:
		return ""
	}
}

func (service *Service) closeSeparateRuntimes(ctx context.Context) {
	if service.separatePool == nil {
		return
	}
	for _, entry := range service.separatePool.snapshot() {
		if removed := service.separatePool.remove(entry.filename); removed != nil {
			service.teardownSeparateEntry(ctx, removed)
		}
	}
}

func separateRuntimeSlug(modelID string, filename string) string {
	slug := strings.TrimSpace(modelID)
	if slug == "" {
		base := filepath.Base(strings.TrimSpace(filename))
		slug = strings.TrimSuffix(base, filepath.Ext(base))
	}
	slug = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, slug)
	if slug == "" {
		return "config"
	}
	return slug
}
