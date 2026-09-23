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

func (webUI *webUIProxy) activeWebUIRoutes(ctx context.Context, definition webUIDefinition) ([]cluster.Route, error) {
	now := time.Now()
	revision := webUI.webUIRegistryRevision()
	if routes, ok := webUI.cachedWebUIRoutes(definition, revision, now); ok {
		return webUI.availableCachedWebUIRoutes(ctx, definition, routes), nil
	}

	webUI.routeMu.Lock()
	defer webUI.routeMu.Unlock()
	if routes, ok := webUI.cachedWebUIRoutes(definition, revision, now); ok {
		return webUI.availableCachedWebUIRoutes(ctx, definition, routes), nil
	}
	routes, err := webUI.discoverActiveWebUIRoutes(ctx, definition)
	if err != nil {
		return nil, err
	}
	if revision != webUI.webUIRegistryRevision() {
		return cloneWebUIRoutes(routes), nil
	}
	byKind := map[string][]cluster.Route{}
	if current := webUI.snapshot.Load(); current != nil && current.registryRevision == revision && now.Before(current.expiresAt) {
		for kind, cached := range current.byKind {
			byKind[kind] = cloneWebUIRoutes(cached)
		}
	}
	byKind[definition.kind] = cloneWebUIRoutes(routes)
	webUI.snapshot.Store(&webUIRouteSnapshot{
		registryRevision: revision,
		expiresAt:        now.Add(webUIRouteSnapshotLifetime),
		byKind:           byKind,
	})
	return webUI.availableCachedWebUIRoutes(ctx, definition, routes), nil
}

func (webUI *webUIProxy) cachedWebUIRoutes(definition webUIDefinition, revision uint64, now time.Time) ([]cluster.Route, bool) {
	snapshot := webUI.snapshot.Load()
	if snapshot == nil || snapshot.registryRevision != revision || !now.Before(snapshot.expiresAt) {
		return nil, false
	}
	routes, ok := snapshot.byKind[definition.kind]
	return cloneWebUIRoutes(routes), ok
}

func (webUI *webUIProxy) availableCachedWebUIRoutes(ctx context.Context, definition webUIDefinition, routes []cluster.Route) []cluster.Route {
	identity := webUI.identity()
	hasLocal := false
	for _, route := range routes {
		if route.NodeID == identity.nodeID || !route.Remote {
			hasLocal = true
			break
		}
	}
	if !hasLocal || webUI.deps.localBackendAvailableForRoute(ctx, definition.backendMode, webUIReadiness(definition.lane)) {
		return cloneWebUIRoutes(routes)
	}
	available := make([]cluster.Route, 0, len(routes))
	for _, route := range routes {
		if route.NodeID != identity.nodeID && route.Remote {
			available = append(available, route)
		}
	}
	return available
}

func (webUI *webUIProxy) invalidate() {
	webUI.routeMu.Lock()
	webUI.snapshot.Store(nil)
	webUI.routeMu.Unlock()
}

func (webUI *webUIProxy) webUIRegistryRevision() uint64 {
	identity := webUI.identity()
	if identity.registry == nil {
		return 0
	}
	return identity.registry.Revision()
}

func cloneWebUIRoutes(routes []cluster.Route) []cluster.Route {
	return append([]cluster.Route{}, routes...)
}
