package proxy

import (
	"net/http"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleSiteAnalytics(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	query, err := routeranalytics.QueryFromValues(r.URL.Query(), time.Now())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response := service.analyticsResponse(r, query)
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeAnalytics(w http.ResponseWriter, r *http.Request) {
	query, err := routeranalytics.QueryFromValues(r.URL.Query(), time.Now())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response := service.localAnalyticsResponse(r, query)
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) analyticsResponse(r *http.Request, query routeranalytics.Query) routeranalytics.Response {
	responses := []routeranalytics.Response{service.localAnalyticsResponse(r, query)}
	if service.clusterRole == cluster.RoleMaster {
		for _, nodeURL := range service.remoteInventoryURLs() {
			var remote routeranalytics.Response
			path := "/router/v1/node/analytics"
			if strings.TrimSpace(r.URL.RawQuery) != "" {
				path += "?" + r.URL.RawQuery
			}
			if err := service.clusterClient.JSON(r.Context(), http.MethodGet, nodeURL, path, nil, &remote); err != nil {
				responses = append(responses, routeranalytics.Response{
					From:        query.StartMS,
					To:          query.EndMS,
					Granularity: routeranalytics.Granularity(query),
					NodeErrors: []routeranalytics.NodeError{{
						NodeURL: nodeURL,
						Error:   err.Error(),
					}},
				})
				continue
			}
			responses = append(responses, remote)
		}
	}
	return routeranalytics.Merge(responses...)
}

func (service *Service) localAnalyticsResponse(r *http.Request, query routeranalytics.Query) routeranalytics.Response {
	if service.analyticsStore == nil {
		return routeranalytics.DisabledResponse(query)
	}
	response, err := service.analyticsStore.Query(r.Context(), query)
	if err != nil {
		return routeranalytics.Response{
			Enabled:     true,
			From:        query.StartMS,
			To:          query.EndMS,
			Granularity: routeranalytics.Granularity(query),
			NodeErrors: []routeranalytics.NodeError{{
				NodeID: service.nodeID,
				Error:  err.Error(),
			}},
		}
	}
	return response
}
