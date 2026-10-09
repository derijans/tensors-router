package proxy

import (
	"context"
	"net/http"

	routeranalytics "tensors-router/internal/analytics"
)

type forwardAnalytics struct {
	analytics *requestAnalytics
	event     routeranalytics.Event
	finalizer routeranalytics.EventFinalizer
}

func (tracking forwardAnalytics) recordFailure(requestContext context.Context, err error) {
	if tracking.analytics == nil {
		return
	}
	tracking.analytics.recordForwardFailure(requestContext, tracking.event, err, tracking.finalizer)
}

func (tracking forwardAnalytics) wrap(response *http.Response) *http.Response {
	if tracking.analytics == nil {
		return response
	}
	return tracking.analytics.withResponse(response, tracking.event, tracking.finalizer)
}
