package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

const (
	siteWebUIProxyPrefix = "/router/webuis/"
	nodeWebUIProxyPrefix = "/router/v1/node/webuis/"
)

func (service *Service) handleSiteWebUIProxy(w http.ResponseWriter, r *http.Request) {
	definition, strippedPath, ok, redirectPath := webUIProxyPath(r.URL.Path, siteWebUIProxyPrefix)
	if redirectPath != "" {
		redirectURL := *r.URL
		redirectURL.Path = redirectPath
		http.Redirect(w, r, redirectURL.String(), http.StatusPermanentRedirect)
		return
	}
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "webui not found")
		return
	}
	if !service.webUISession.isEnabled(definition.kind) {
		openai.WriteError(w, http.StatusNotFound, "not_found", "webui is not enabled")
		return
	}
	route, release, ok := service.acquireWebUIProxyRoute(r.Context(), definition)
	if !ok {
		openai.WriteError(w, http.StatusServiceUnavailable, "backend_error", "webui has no active compatible backend")
		return
	}
	defer release()
	response, err := service.forwardWebUIProxy(r.Context(), r, definition, strippedPath, route, siteWebUIProxyPrefix)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	if err := writeWebUIProxyResponse(w, response); err != nil {
		return
	}
}

func (service *Service) handleNodeWebUIProxy(w http.ResponseWriter, r *http.Request) {
	definition, strippedPath, ok, _ := webUIProxyPath(r.URL.Path, nodeWebUIProxyPrefix)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "webui not found")
		return
	}
	route, release, ok := service.acquireLocalWebUIProxyRoute(r.Context(), definition)
	if !ok {
		openai.WriteError(w, http.StatusServiceUnavailable, "backend_error", "webui has no active compatible backend")
		return
	}
	defer release()
	response, err := service.forwardLocalWebUIProxy(r.Context(), r, definition, strippedPath, route, nodeWebUIProxyPrefix)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	if err := writeWebUIProxyResponse(w, response); err != nil {
		return
	}
}

func (service *Service) acquireWebUIProxyRoute(ctx context.Context, definition webUIDefinition) (cluster.Route, func(), bool) {
	routes, err := service.activeWebUIRoutes(ctx, definition)
	if err != nil || len(routes) == 0 {
		return cluster.Route{}, func() {}, false
	}
	if service.registry != nil {
		return service.registry.AcquireWebUI(definition.kind, routes)
	}
	return routes[0], func() {}, true
}

func (service *Service) acquireLocalWebUIProxyRoute(ctx context.Context, definition webUIDefinition) (cluster.Route, func(), bool) {
	routes, err := service.localActiveWebUIRoutes(ctx, definition)
	if err != nil || len(routes) == 0 {
		return cluster.Route{}, func() {}, false
	}
	return routes[0], func() {}, true
}

func (service *Service) activeWebUIRoutes(ctx context.Context, definition webUIDefinition) ([]cluster.Route, error) {
	routes, err := service.localActiveWebUIRoutes(ctx, definition)
	if err != nil {
		return nil, err
	}
	if service.clusterRole != cluster.RoleMaster {
		return routes, nil
	}
	remoteRoutes := service.remoteActiveWebUIRoutes(ctx, definition)
	routes = append(routes, remoteRoutes...)
	return routes, nil
}

func (service *Service) localActiveWebUIRoutes(ctx context.Context, definition webUIDefinition) ([]cluster.Route, error) {
	if !service.localBackendAvailableForRoute(ctx, definition.backendMode, webUIReadiness(definition.lane)) {
		return nil, nil
	}
	entries, err := service.localWebUIs()
	if err != nil {
		return nil, err
	}
	entry, ok := webUIEntryByID(entries, definition.kind)
	if !ok || !entry.Active {
		return nil, nil
	}
	return webUIRoutesFromEntry(definition, entry, false), nil
}

func (service *Service) remoteActiveWebUIRoutes(ctx context.Context, definition webUIDefinition) []cluster.Route {
	routes := []cluster.Route{}
	for _, nodeURL := range service.remoteInventoryURLs() {
		var remote WebUICatalogResponse
		if err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/site/webuis", nil, &remote); err != nil {
			continue
		}
		for _, entry := range remote.Data {
			if entry.ID != definition.kind || !entry.Active {
				continue
			}
			for index := range entry.CompatibleModels {
				if entry.CompatibleModels[index].NodeURL == "" {
					entry.CompatibleModels[index].NodeURL = firstNonEmpty(entry.NodeURL, nodeURL)
				}
			}
			routes = append(routes, webUIRoutesFromEntry(definition, entry, true)...)
		}
	}
	return routes
}

func webUIRoutesFromEntry(definition webUIDefinition, entry WebUIEntry, remote bool) []cluster.Route {
	routes := []cluster.Route{}
	for _, model := range entry.CompatibleModels {
		if !model.Active {
			continue
		}
		routes = append(routes, cluster.Route{
			PublicID:      model.ModelID,
			LocalID:       firstNonEmpty(model.LocalID, model.ModelID),
			PublicImageID: model.ImageID,
			LocalImageID:  firstNonEmpty(model.LocalImageID, model.ImageID),
			Filename:      model.Filename,
			NodeID:        model.NodeID,
			NodeURL:       firstNonEmpty(model.NodeURL, entry.NodeURL),
			Remote:        remote,
			Lane:          definition.lane,
			BackendMode:   definition.backendMode,
		})
	}
	return routes
}

func (service *Service) forwardWebUIProxy(ctx context.Context, original *http.Request, definition webUIDefinition, strippedPath string, route cluster.Route, publicPrefix string) (*http.Response, error) {
	if route.Remote {
		return service.forwardRemoteWebUIProxy(ctx, original, definition, strippedPath, route)
	}
	return service.forwardLocalWebUIProxy(ctx, original, definition, strippedPath, route, publicPrefix)
}

func (service *Service) forwardLocalWebUIProxy(ctx context.Context, original *http.Request, definition webUIDefinition, strippedPath string, route cluster.Route, publicPrefix string) (*http.Response, error) {
	runtime, release, err := service.lockWebUIRoute(ctx, definition, route)
	if err != nil {
		return nil, err
	}
	response, err := service.forwardWebUIRequest(ctx, original, runtime.backend.URL(), webUIBackendPath(definition, strippedPath))
	if err != nil {
		release()
		return nil, err
	}
	response = responseWithRelease(response, release)
	rewriteWebUIResponseLocation(response, runtime.backend.URL(), definition, publicPrefix, strippedPath)
	return response, nil
}

func (service *Service) forwardRemoteWebUIProxy(ctx context.Context, original *http.Request, definition webUIDefinition, strippedPath string, route cluster.Route) (*http.Response, error) {
	baseURL, err := service.clusterClient.AuthorizedBaseURL(route.NodeURL)
	if err != nil {
		return nil, err
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	targetPath := nodeWebUIProxyURL(definition, strippedPath)
	response, err := service.forwardWebUIRequest(ctx, original, parsedBaseURL, targetPath)
	if err != nil {
		return nil, err
	}
	rewriteWebUIPathPrefixLocation(response, parsedBaseURL, nodeWebUIProxyURL(definition, "/"), siteWebUIProxyURL(definition, "/"))
	return response, nil
}

func (service *Service) lockWebUIRoute(ctx context.Context, definition webUIDefinition, route cluster.Route) (*backendRuntime, func(), error) {
	modelID := route.PublicID
	if definition.lane == cluster.RouteLaneImage && strings.TrimSpace(route.PublicImageID) != "" {
		modelID = route.PublicImageID
	}
	runtime, release, _, err := service.acquireModelConfigForBackendMode(route.BackendMode, ctx, modelID, route.Filename, webUIReadiness(definition.lane), false)
	if err != nil {
		return nil, nil, err
	}
	return runtime, release, nil
}

func (service *Service) forwardWebUIRequest(ctx context.Context, original *http.Request, baseURL *url.URL, path string) (*http.Response, error) {
	body, err := readProxyRequestBody(original)
	if err != nil {
		return nil, err
	}
	target := *baseURL
	target.Path = joinPath(target.Path, path)
	target.RawQuery = original.URL.RawQuery
	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyRequestHeaders(request.Header, original.Header)
	if service.clusterToken != "" && strings.HasPrefix(path, nodeWebUIProxyPrefix) {
		request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	}
	request.Host = target.Host
	client := *service.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client.Do(request)
}

func readProxyRequestBody(original *http.Request) ([]byte, error) {
	if original.Body == nil {
		return nil, nil
	}
	defer original.Body.Close()
	return io.ReadAll(original.Body)
}

func writeWebUIProxyResponse(w http.ResponseWriter, response *http.Response) error {
	return writeProxyResponse(w, response, "", false)
}

func rewriteWebUIResponseLocation(response *http.Response, baseURL *url.URL, definition webUIDefinition, publicPrefix string, strippedPath string) {
	if response == nil {
		return
	}
	location := response.Header.Get("Location")
	if location == "" {
		return
	}
	backendPrefix := joinPath(baseURL.Path, definition.path)
	if webUIBackendAPIPath(definition, strippedPath) {
		backendPrefix = firstNonEmpty(baseURL.Path, "/")
	}
	if rewritten, ok := rewriteLocationPath(location, baseURL, backendPrefix, webUIProxyURL(publicPrefix, definition, "/")); ok {
		response.Header.Set("Location", rewritten)
	}
}

func rewriteWebUIPathPrefixLocation(response *http.Response, baseURL *url.URL, sourcePrefix string, targetPrefix string) {
	if response == nil {
		return
	}
	location := response.Header.Get("Location")
	if location == "" {
		return
	}
	source := joinPath(baseURL.Path, sourcePrefix)
	if rewritten, ok := rewriteLocationPath(location, baseURL, source, targetPrefix); ok {
		response.Header.Set("Location", rewritten)
	}
}

func rewriteLocationPath(location string, baseURL *url.URL, sourcePrefix string, targetPrefix string) (string, bool) {
	parsed, err := url.Parse(location)
	if err != nil {
		return location, false
	}
	if parsed.IsAbs() {
		if !strings.EqualFold(parsed.Scheme, baseURL.Scheme) || !strings.EqualFold(parsed.Host, baseURL.Host) {
			return location, false
		}
		rewrittenPath, ok := replaceWebUILocationPath(parsed.Path, sourcePrefix, targetPrefix)
		if !ok {
			return location, false
		}
		parsed.Scheme = ""
		parsed.Host = ""
		parsed.User = nil
		parsed.Path = rewrittenPath
		return parsed.String(), true
	}
	if strings.HasPrefix(location, "//") {
		return location, false
	}
	if !strings.HasPrefix(parsed.Path, "/") {
		return location, false
	}
	rewrittenPath, ok := replaceWebUILocationPath(parsed.Path, sourcePrefix, targetPrefix)
	if !ok {
		return location, false
	}
	parsed.Path = rewrittenPath
	return parsed.String(), true
}

func replaceWebUILocationPath(path string, sourcePrefix string, targetPrefix string) (string, bool) {
	source := strings.TrimRight(sourcePrefix, "/")
	if source == "" {
		source = "/"
	}
	target := strings.TrimRight(targetPrefix, "/")
	switch {
	case source == "/":
		return target + "/" + strings.TrimLeft(path, "/"), true
	case path == source:
		return target + "/", true
	case strings.HasPrefix(path, source+"/"):
		return target + "/" + strings.TrimLeft(strings.TrimPrefix(path, source), "/"), true
	default:
		return "", false
	}
}

func webUIProxyPath(path string, prefix string) (webUIDefinition, string, bool, string) {
	if !strings.HasPrefix(path, prefix) {
		return webUIDefinition{}, "", false, ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	kind, rest, hasRest := strings.Cut(remainder, "/")
	definition, ok := webUIDefinitionByKind(kind)
	if !ok {
		return webUIDefinition{}, "", false, ""
	}
	if !hasRest {
		return definition, "", false, webUIProxyURL(prefix, definition, "/")
	}
	return definition, "/" + rest, true, ""
}

func webUIBackendPath(definition webUIDefinition, strippedPath string) string {
	if webUIBackendAPIPath(definition, strippedPath) {
		return strippedPath
	}
	return joinPath(definition.path, strippedPath)
}

func webUIBackendAPIPath(definition webUIDefinition, path string) bool {
	switch definition.kind {
	case "sdcpp":
		return webUIPathHasPrefix(path, "/sdcpp/v1/", "/sdapi/v1/", "/v1/images/") || path == "/v1/models"
	case "llama":
		return webUIPathHasPrefix(path, "/v1/", "/api/v1/") ||
			webUIPathIs(path, "/completion", "/chat", "/infill", "/embedding", "/embeddings", "/rerank", "/tokenize", "/detokenize", "/props", "/slots", "/metrics", "/health")
	case "kobold-lite", "kobold-lcpp":
		return webUIPathHasPrefix(path, "/v1/", "/api/v1/", "/api/extra/") ||
			webUIPathIs(path, "/api/generate", "/api/chat", "/api/show", "/api/tags", "/api/ps", "/api/version")
	case "kobold-sd":
		return webUIPathHasPrefix(path, "/sdapi/v1/", "/v1/images/", "/history/", "/view/", "/object_info/", "/upload/image") ||
			webUIPathIs(path, "/prompt", "/queue", "/history", "/view", "/object_info", "/system_stats", "/interrupt")
	case "kobold-music":
		return webUIPathHasPrefix(path, "/api/extra/music/")
	default:
		return false
	}
}

func webUIPathHasPrefix(path string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) || strings.TrimRight(prefix, "/") == path {
			return true
		}
	}
	return false
}

func webUIPathIs(path string, values ...string) bool {
	for _, value := range values {
		if path == value {
			return true
		}
	}
	return false
}

func siteWebUIProxyURL(definition webUIDefinition, strippedPath string) string {
	return webUIProxyURL(siteWebUIProxyPrefix, definition, strippedPath)
}

func nodeWebUIProxyURL(definition webUIDefinition, strippedPath string) string {
	return webUIProxyURL(nodeWebUIProxyPrefix, definition, strippedPath)
}

func webUIProxyURL(prefix string, definition webUIDefinition, strippedPath string) string {
	path := strings.TrimRight(prefix, "/") + "/" + definition.kind + "/"
	return path + strings.TrimLeft(strippedPath, "/")
}
