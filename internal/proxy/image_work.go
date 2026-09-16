package proxy

import (
	"net/http"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/schedulingcost"
)

// imageWorkHint prices the request before it is queued, reusing the same
// extraction that fills the analytics row the cost model was fitted from. A request
// whose body was never buffered yields a zero hint rather than a guess.
func imageWorkHint(r *http.Request, body []byte) requestWorkHint {
	if len(body) == 0 {
		return requestWorkHint{}
	}
	event := routeranalytics.Event{Section: routeranalytics.SectionImage}
	routeranalytics.ApplyRequest(&event, r.URL.Path, body, r.Header.Get("Content-Type"))
	return requestWorkHint{Work: schedulingcost.ImageWork(routeranalytics.ImageWork(event))}
}
