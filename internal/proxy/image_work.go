package proxy

import (
	"net/http"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
)

// imageRouteHint prices the request before it is dispatched, reusing the same
// extraction that fills the analytics row the cost model was fitted from.
//
// A request whose body was never buffered, or that does not state its size,
// yields a zero hint. That is not a fallback to a guess: an unpriced request skips
// cost ordering entirely and takes the existing rotation.
func imageRouteHint(r *http.Request, body []byte) cluster.RouteHint {
	if len(body) == 0 {
		return cluster.RouteHint{}
	}
	event := routeranalytics.Event{Section: routeranalytics.SectionImage}
	routeranalytics.ApplyRequest(&event, r.URL.Path, body, r.Header.Get("Content-Type"))
	return cluster.RouteHint{Work: routeranalytics.ImageWork(event)}
}
