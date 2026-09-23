package proxy

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/hardware"
	"tensors-router/internal/recipes"
)

const analyticsRequestMetadataLimit = 1 << 20

type requestAnalytics struct {
	store        *routeranalytics.Store
	vramEnabled  bool
	vramSource   hardware.VRAMSource
	vramSampler  *hardware.VRAMSampler
	vramInterval time.Duration
	nodeID       string
	loadSection  func(configFilename string, readiness backendReadiness) string
}

func (analytics *requestAnalytics) newEvent(started time.Time, r *http.Request, body []byte, modelID string, section string, backendMode string) routeranalytics.Event {
	event := routeranalytics.Event{
		NodeID:       analytics.nodeID,
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

func (analytics *requestAnalytics) withResponse(response *http.Response, event routeranalytics.Event, finalizers ...routeranalytics.EventFinalizer) *http.Response {
	if analytics.store == nil {
		return response
	}
	if response == nil {
		analytics.recordFailure(event, http.StatusBadGateway, finalizers...)
		return response
	}
	event.StatusCode = response.StatusCode
	event.Success = response.StatusCode >= 200 && response.StatusCode < 300
	event.AudioLanguage = response.Header.Get("X-Tensors-Audio-Language")
	event.AudioTask = response.Header.Get("X-Tensors-Audio-Task")
	if value, err := strconv.ParseFloat(response.Header.Get("X-Tensors-Audio-Duration"), 64); err == nil {
		event.AudioSeconds = value
	}
	response.Header.Del("X-Tensors-Audio-Language")
	response.Header.Del("X-Tensors-Audio-Task")
	response.Header.Del("X-Tensors-Audio-Duration")
	if response.Body == nil {
		analytics.recordFinished(event, finalizers...)
		return response
	}
	response.Body = routeranalytics.NewResponseObserver(analytics.store, event, response.Header.Get("Content-Type"), response.Body, finalizers...)
	return response
}

func (analytics *requestAnalytics) recordFailure(event routeranalytics.Event, statusCode int, finalizers ...routeranalytics.EventFinalizer) {
	if analytics.store == nil {
		return
	}
	event.StatusCode = statusCode
	event.Success = false
	analytics.recordFinished(event, finalizers...)
}

func (analytics *requestAnalytics) recordFinished(event routeranalytics.Event, finalizers ...routeranalytics.EventFinalizer) {
	if analytics.store == nil {
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
	analytics.store.Record(event)
}

func stampLoadedModel(runtime *backendRuntime) routeranalytics.EventFinalizer {
	if runtime == nil || runtime.state == nil {
		return nil
	}
	return func(event *routeranalytics.Event) {
		if event == nil || event.ModelID != "" {
			return
		}
		event.ModelID, event.ConfigFilename = runtime.state.loadedModel()
	}
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
