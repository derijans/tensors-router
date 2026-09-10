package proxy

import (
	"net/http"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
)

func (service *Service) webUIProxyResponseWithAnalytics(response *http.Response, started time.Time, r *http.Request, definition webUIDefinition, strippedPath string, route cluster.Route) *http.Response {
	if service.analyticsStore == nil || route.Remote || !webUIInferencePath(definition, strippedPath) {
		return response
	}
	section := webUIAnalyticsSection(definition, strippedPath)
	modelID := route.LocalID
	if section == routeranalytics.SectionImage && route.LocalImageID != "" {
		modelID = route.LocalImageID
	}
	event := service.newAnalyticsEvent(started, r, nil, modelID, section, definition.backendMode)
	event.Route = routeranalytics.RouteClass(strippedPath)
	event.ConfigFilename = route.Filename
	return service.responseWithAnalytics(response, event)
}

func webUIInferencePath(definition webUIDefinition, path string) bool {
	if isTextInferencePath(path) || isImageGenerationPath(path) {
		return true
	}
	switch definition.kind {
	case "llama":
		return path == "/completion" || path == "/chat" || path == "/infill" ||
			path == "/embedding" || path == "/embeddings" || path == "/rerank"
	case "whispercpp":
		return path == "/inference"
	case "kobold-music":
		return path == "/api/extra/music/generate"
	default:
		return isVoicePath(path)
	}
}

func isImageGenerationPath(path string) bool {
	switch path {
	case "/prompt",
		"/sdapi/v1/txt2img",
		"/sdapi/v1/img2img",
		"/sdapi/v1/extra-single-image",
		"/sdapi/v1/upscale",
		"/v1/images/generations",
		"/v1/images/edits",
		"/sdcpp/v1/img_gen",
		"/sdcpp/v1/vid_gen":
		return true
	default:
		return false
	}
}

func webUIAnalyticsSection(definition webUIDefinition, path string) string {
	if isImageGenerationPath(path) {
		return routeranalytics.SectionImage
	}
	if isMusicPath(path) || definition.lane == cluster.RouteLaneMusic {
		return routeranalytics.SectionMusic
	}
	if isVoicePath(path) || path == "/inference" || definition.lane == cluster.RouteLaneVoice {
		return routeranalytics.SectionVoice
	}
	return textAnalyticsSection(path)
}
