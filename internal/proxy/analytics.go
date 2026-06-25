package proxy

import (
	"net/http"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/recipes"
)

func (service *Service) newAnalyticsEvent(started time.Time, r *http.Request, body []byte, modelID string, section string, backendMode string) routeranalytics.Event {
	event := routeranalytics.Event{
		NodeID:      service.nodeID,
		ModelID:     strings.TrimSpace(modelID),
		Section:     strings.TrimSpace(section),
		BackendMode: strings.TrimSpace(backendMode),
		Route:       routeranalytics.RouteClass(r.URL.Path),
		StartedAt:   started,
	}
	routeranalytics.ApplyRequest(&event, r.URL.Path, body, r.Header.Get("Content-Type"))
	return event
}

func (service *Service) responseWithAnalytics(response *http.Response, event routeranalytics.Event) *http.Response {
	if service.analyticsStore == nil {
		return response
	}
	if response == nil {
		service.recordAnalyticsFailure(event, http.StatusBadGateway)
		return response
	}
	event.StatusCode = response.StatusCode
	event.Success = response.StatusCode >= 200 && response.StatusCode < 300
	if response.Body == nil {
		service.recordAnalyticsFinished(event)
		return response
	}
	response.Body = routeranalytics.NewResponseObserver(service.analyticsStore, event, response.Header.Get("Content-Type"), response.Body)
	return response
}

func (service *Service) recordAnalyticsFailure(event routeranalytics.Event, statusCode int) {
	if service.analyticsStore == nil {
		return
	}
	event.StatusCode = statusCode
	event.Success = false
	service.recordAnalyticsFinished(event)
}

func (service *Service) recordAnalyticsFinished(event routeranalytics.Event) {
	if service.analyticsStore == nil {
		return
	}
	event.FinishedAt = time.Now()
	if event.StartedAt.IsZero() {
		event.StartedAt = event.FinishedAt
	}
	event.DurationMS = event.FinishedAt.Sub(event.StartedAt).Milliseconds()
	service.analyticsStore.Record(event)
}

func textAnalyticsSection(path string) string {
	if isEmbeddingsPath(path) {
		return routeranalytics.SectionEmbed
	}
	return routeranalytics.SectionLLM
}

func audioAnalyticsSection(lane string) string {
	if lane == recipes.KindMusic {
		return routeranalytics.SectionMusic
	}
	return routeranalytics.SectionVoice
}
