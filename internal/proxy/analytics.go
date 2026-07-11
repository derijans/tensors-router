package proxy

import (
	"net/http"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/recipes"
)

const analyticsRequestMetadataLimit = 1 << 20

func (service *Service) newAnalyticsEvent(started time.Time, r *http.Request, body []byte, modelID string, section string, backendMode string) routeranalytics.Event {
	event := routeranalytics.Event{
		NodeID:       service.nodeID,
		ModelID:      strings.TrimSpace(modelID),
		Section:      strings.TrimSpace(section),
		BackendMode:  strings.TrimSpace(backendMode),
		Route:        routeranalytics.RouteClass(r.URL.Path),
		StartedAt:    started,
		RequestBytes: int64(len(body)),
	}
	if len(body) <= analyticsRequestMetadataLimit {
		routeranalytics.ApplyRequest(&event, r.URL.Path, body, r.Header.Get("Content-Type"))
	}
	return event
}

func (service *Service) responseWithAnalytics(response *http.Response, event routeranalytics.Event, finalizers ...analyticsEventFinalizer) *http.Response {
	if service.analyticsStore == nil {
		return response
	}
	if response == nil {
		service.recordAnalyticsFailure(event, http.StatusBadGateway, finalizers...)
		return response
	}
	event.StatusCode = response.StatusCode
	event.Success = response.StatusCode >= 200 && response.StatusCode < 300
	if response.Body == nil {
		service.recordAnalyticsFinished(event, finalizers...)
		return response
	}
	response.Body = routeranalytics.NewResponseObserver(service.analyticsStore, event, response.Header.Get("Content-Type"), response.Body, finalizers...)
	return response
}

func (service *Service) recordAnalyticsFailure(event routeranalytics.Event, statusCode int, finalizers ...analyticsEventFinalizer) {
	if service.analyticsStore == nil {
		return
	}
	event.StatusCode = statusCode
	event.Success = false
	service.recordAnalyticsFinished(event, finalizers...)
}

func (service *Service) recordAnalyticsFinished(event routeranalytics.Event, finalizers ...analyticsEventFinalizer) {
	if service.analyticsStore == nil {
		return
	}
	event.FinishedAt = time.Now()
	if event.StartedAt.IsZero() {
		event.StartedAt = event.FinishedAt
	}
	event.DurationMS = event.FinishedAt.Sub(event.StartedAt).Milliseconds()
	for _, finalizer := range finalizers {
		if finalizer != nil {
			finalizer(&event)
		}
	}
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
