package proxy

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/metrics"
)

const (
	metricsPrefix       = "tensors_router_"
	metricsModelNone    = "none"
	loadResultSucceeded = "ok"
	loadResultFailed    = "error"
)

type routerMetrics struct {
	registry            *metrics.Registry
	knownModel          func(modelID string) bool
	requests            *metrics.CounterVec
	requestSeconds      *metrics.HistogramVec
	modelLoadSeconds    *metrics.HistogramVec
	loadErrors          *metrics.CounterVec
	transportRejections *metrics.CounterVec
	idleUnloads         *metrics.CounterVec
}

func newRouterMetrics(knownModel func(modelID string) bool) *routerMetrics {
	registry := metrics.NewRegistry()
	return &routerMetrics{
		registry:            registry,
		knownModel:          knownModel,
		requests:            registry.NewCounter(metricsPrefix+"requests_total", "Inference requests finished, by route class, model and HTTP status.", "route", "model", "status"),
		requestSeconds:      registry.NewHistogram(metricsPrefix+"request_duration_seconds", "Time from request start until the response body closed.", metrics.DurationBuckets, "route"),
		modelLoadSeconds:    registry.NewHistogram(metricsPrefix+"model_load_duration_seconds", "Time a backend took to load a config.", metrics.DurationBuckets, "backend_mode", "result"),
		loadErrors:          registry.NewCounter(metricsPrefix+"model_load_errors_total", "Recorded load errors and warnings, by phase and severity.", "phase", "severity"),
		transportRejections: registry.NewCounter(metricsPrefix+"transport_rejections_total", "Requests refused because the transport memory budget was exhausted."),
		idleUnloads:         registry.NewCounter(metricsPrefix+"idle_unloads_total", "Runtimes unloaded by router_idle_unload_seconds.", "backend_mode"),
	}
}

func (recorder *routerMetrics) observeResponse(response *http.Response, event routeranalytics.Event) *http.Response {
	if recorder == nil {
		return response
	}
	if response.Body == nil {
		recorder.requestFinished(event, response.StatusCode)
		return response
	}
	response.Body = &metricsObservedBody{ReadCloser: response.Body, finish: func() { recorder.requestFinished(event, response.StatusCode) }}
	return response
}

func (recorder *routerMetrics) requestFinished(event routeranalytics.Event, statusCode int) {
	if recorder == nil {
		return
	}
	recorder.requests.Inc(event.Route, recorder.modelLabel(event.ModelID), strconv.Itoa(statusCode))
	if !event.StartedAt.IsZero() {
		recorder.requestSeconds.Observe(time.Since(event.StartedAt).Seconds(), event.Route)
	}
}

func (recorder *routerMetrics) modelLabel(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	switch {
	case modelID == "":
		return metricsModelNone
	case recorder.knownModel != nil && recorder.knownModel(modelID):
		return modelID
	default:
		return metrics.OverflowLabel
	}
}

func (recorder *routerMetrics) modelLoadFinished(backendMode string, elapsed time.Duration, err error) {
	if recorder == nil {
		return
	}
	result := loadResultSucceeded
	if err != nil {
		result = loadResultFailed
	}
	recorder.modelLoadSeconds.Observe(elapsed.Seconds(), backendMode, result)
}

func (recorder *routerMetrics) loadErrorRecorded(phase loaderrors.Phase, severity loaderrors.Severity) {
	if recorder == nil {
		return
	}
	if severity != loaderrors.SeverityWarning {
		severity = loaderrors.SeverityError
	}
	recorder.loadErrors.Inc(string(phase), string(severity))
}

func (recorder *routerMetrics) transportRejected() {
	if recorder == nil {
		return
	}
	recorder.transportRejections.Inc()
}

func (recorder *routerMetrics) idleUnloaded(backendMode string) {
	if recorder == nil {
		return
	}
	recorder.idleUnloads.Inc(backendMode)
}

type metricsObservedBody struct {
	io.ReadCloser
	once   sync.Once
	finish func()
}

func (body *metricsObservedBody) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(body.finish)
	return err
}
