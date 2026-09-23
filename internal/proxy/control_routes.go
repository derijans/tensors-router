package proxy

import (
	"net/http"

	"tensors-router/internal/proxy/routing"
)

func (service *Service) controlRoutes() []routing.Route {
	return []routing.Route{
		routing.Prefix(routing.AnyMethod, "/router/v1/vllm/", service.handleVLLMAdmin),
		routing.Exact(http.MethodGet, "/router/v1/models", service.handleRouterModels),
		routing.Exact(http.MethodGet, "/router/v1/version", service.handleRouterVersion),
		routing.Exact(http.MethodPost, "/router/v1/load", service.handleRouterLoad),
		routing.Exact(http.MethodPost, "/router/v1/unload", service.handleRouterUnload),
		routing.Exact(http.MethodPost, "/router/v1/shutdown", service.handleRouterShutdown),
	}
}
