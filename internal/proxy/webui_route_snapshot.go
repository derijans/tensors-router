package proxy

import (
	"context"
	"time"

	"tensors-router/internal/cluster"
)

const webUIRouteSnapshotLifetime = 5 * time.Second

type webUIRouteSnapshot struct {
	registryRevision uint64
	expiresAt        time.Time
	byKind           map[string][]cluster.Route
}

func (service *Service) activeWebUIRoutes(ctx context.Context, definition webUIDefinition) ([]cluster.Route, error) {
	now := time.Now()
	revision := service.webUIRegistryRevision()
	if routes, ok := service.cachedWebUIRoutes(definition, revision, now); ok {
		return service.availableCachedWebUIRoutes(ctx, definition, routes), nil
	}

	service.webUIRouteMu.Lock()
	defer service.webUIRouteMu.Unlock()
	if routes, ok := service.cachedWebUIRoutes(definition, revision, now); ok {
		return service.availableCachedWebUIRoutes(ctx, definition, routes), nil
	}
	routes, err := service.discoverActiveWebUIRoutes(ctx, definition)
	if err != nil {
		return nil, err
	}
	if revision != service.webUIRegistryRevision() {
		return cloneWebUIRoutes(routes), nil
	}
	byKind := map[string][]cluster.Route{}
	if current := service.webUIRoutes.Load(); current != nil && current.registryRevision == revision && now.Before(current.expiresAt) {
		for kind, cached := range current.byKind {
			byKind[kind] = cloneWebUIRoutes(cached)
		}
	}
	byKind[definition.kind] = cloneWebUIRoutes(routes)
	service.webUIRoutes.Store(&webUIRouteSnapshot{
		registryRevision: revision,
		expiresAt:        now.Add(webUIRouteSnapshotLifetime),
		byKind:           byKind,
	})
	return service.availableCachedWebUIRoutes(ctx, definition, routes), nil
}

func (service *Service) cachedWebUIRoutes(definition webUIDefinition, revision uint64, now time.Time) ([]cluster.Route, bool) {
	snapshot := service.webUIRoutes.Load()
	if snapshot == nil || snapshot.registryRevision != revision || !now.Before(snapshot.expiresAt) {
		return nil, false
	}
	routes, ok := snapshot.byKind[definition.kind]
	return cloneWebUIRoutes(routes), ok
}

func (service *Service) availableCachedWebUIRoutes(ctx context.Context, definition webUIDefinition, routes []cluster.Route) []cluster.Route {
	hasLocal := false
	for _, route := range routes {
		if route.NodeID == service.nodeID || !route.Remote {
			hasLocal = true
			break
		}
	}
	if !hasLocal || service.localBackendAvailableForRoute(ctx, definition.backendMode, webUIReadiness(definition.lane)) {
		return cloneWebUIRoutes(routes)
	}
	available := make([]cluster.Route, 0, len(routes))
	for _, route := range routes {
		if route.NodeID != service.nodeID && route.Remote {
			available = append(available, route)
		}
	}
	return available
}

func (service *Service) invalidateWebUIRoutes() {
	service.webUIRouteMu.Lock()
	service.webUIRoutes.Store(nil)
	service.webUIRouteMu.Unlock()
}

func (service *Service) webUIRegistryRevision() uint64 {
	if service.registry == nil {
		return 0
	}
	return service.registry.Revision()
}

func cloneWebUIRoutes(routes []cluster.Route) []cluster.Route {
	return append([]cluster.Route{}, routes...)
}
