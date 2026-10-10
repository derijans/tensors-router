package proxy

import (
	"context"
	"fmt"
	"net/http"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleRegistryImageRequest(w http.ResponseWriter, r *http.Request, body []byte, publicImageID string) bool {
	activeConfigFilename := service.imageCatalogConfigSelector()
	model, hasImageModel := service.registry.ImageModel(publicImageID, activeConfigFilename)
	if !hasImageModel {
		return false
	}
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return true
	}
	acquired, ok := service.acquireRegistryImageRoute(w, r, publicImageID, model, modelBackendMode, activeConfigFilename)
	if !ok {
		return true
	}
	request := rewriteImageRequest(r, publicImageID, acquired.route.LocalImageID)
	requestBody := rewriteImageRequestBody(body, publicImageID, acquired.route.LocalImageID, r)
	forwarded, routeBackendMode, ok := service.forwardRegistryImageRequest(w, r, request, body, requestBody, acquired)
	defer forwarded.complete()
	if !ok {
		return true
	}
	jobBackendMode := modelBackendMode
	if !acquired.route.Remote {
		jobBackendMode = routeBackendMode
	}
	service.writeRegistryImageResponse(w, r, forwarded, acquired, jobBackendMode)
	return true
}

func (service *Service) acquireRegistryImageRoute(w http.ResponseWriter, r *http.Request, publicImageID string, model cluster.Model, modelBackendMode string, activeConfigFilename string) (acquiredRegistryRoute, bool) {
	route, release, ok := service.registry.AcquireImage(publicImageID, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readinessImage), activeConfigFilename)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("image model %q has no available replicas", publicImageID))
		return acquiredRegistryRoute{}, false
	}
	if borrowedRequestMustStayOnThisNode(r, route) {
		release()
		writeOffloadReturned(w)
		return acquiredRegistryRoute{}, false
	}
	return acquiredRegistryRoute{publicID: publicImageID, model: model, route: route, release: release}, true
}

func (service *Service) forwardRegistryImageRequest(w http.ResponseWriter, r *http.Request, request *http.Request, body []byte, requestBody []byte, acquired acquiredRegistryRoute) (registryForward, string, bool) {
	forwarded := registryForward{complete: func() {}}
	route := acquired.route
	if route.Remote {
		response, err := service.forwardRemote(r.Context(), request, requestBody, route)
		if err != nil {
			acquired.release()
			service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
			return forwarded, "", false
		}
		forwarded.response = response
		return forwarded, "", true
	}
	routeBackendMode, ok := service.admitLocalRegistryImageRequest(w, r, acquired)
	if !ok {
		return forwarded, "", false
	}
	complete, handled := service.serveImageThroughLendingQueue(w, r, request, body, requestBody, acquired)
	if handled {
		return forwarded, "", false
	}
	if complete != nil {
		forwarded.complete = complete
	}
	if err := service.loadPrimaryTextRuntimeForImage(r.Context(), acquired, routeBackendMode); err != nil {
		acquired.release()
		service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
		return forwarded, "", false
	}
	tracking := forwardAnalytics{analytics: service.analytics}
	tracking.event = service.analytics.newEvent(time.Now(), request, requestBody, route.LocalImageID, routeranalytics.SectionImage, routeBackendMode)
	target := backendForwardTarget{modelID: route.PublicImageID, configFilename: route.Filename, hasModel: true, readiness: readinessImage, mode: routeBackendMode}
	response, finalizer, err := service.forwardWithFallbackObserved(r.Context(), request, requestBody, target)
	tracking.finalizer = finalizer
	forwarded.tracking = tracking
	if err != nil {
		acquired.release()
		tracking.recordFailure(r.Context(), err)
		service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
		return forwarded, "", false
	}
	forwarded.response = response
	return forwarded, routeBackendMode, true
}

func (service *Service) admitLocalRegistryImageRequest(w http.ResponseWriter, r *http.Request, acquired acquiredRegistryRoute) (string, bool) {
	routeBackendMode, err := service.clusterRouteBackendMode(acquired.route, acquired.model)
	if err != nil {
		acquired.release()
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return "", false
	}
	if service.borrowedRequestWouldLoadWithoutPermission(r, routeBackendMode, readinessImage, acquired.route.Filename) {
		acquired.release()
		writeOffloadReturned(w)
		return "", false
	}
	return routeBackendMode, true
}

func (service *Service) loadPrimaryTextRuntimeForImage(ctx context.Context, acquired acquiredRegistryRoute, routeBackendMode string) error {
	if routeBackendMode != BackendModeLlamaSDCPP || !clusterModelNeedsPrimaryTextRuntime(acquired.model) {
		return nil
	}
	return service.loadLocalRuntimeForRequest(ctx, routeBackendMode, acquired.route.PublicID, acquired.route.Filename, readinessText)
}

func (service *Service) writeRegistryImageResponse(w http.ResponseWriter, r *http.Request, forwarded registryForward, acquired acquiredRegistryRoute, jobBackendMode string) {
	response := forwarded.response
	route := acquired.route
	if isSdcppJobSubmissionPath(r.URL.Path) {
		response = service.responseWithSdcppJobTracking(response, sdcppJobTarget{
			publicImageID:  acquired.publicID,
			configFilename: route.Filename,
			backendMode:    jobBackendMode,
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
		})
	}
	response = responseWithRelease(response, acquired.release)
	response = forwarded.tracking.wrap(response)
	_ = service.writeProxyResponse(w, response, acquired.publicID, true)
}

func (service *Service) serveImageThroughLendingQueue(w http.ResponseWriter, r *http.Request, request *http.Request, body []byte, requestBody []byte, acquired acquiredRegistryRoute) (complete func(), handled bool) {
	modelID := acquired.route.LocalImageID
	if !service.scheduler.queuesForLending(cluster.RouteLaneImage, acquired.route.NodeID, modelID) {
		return nil, false
	}
	admission, queueErr := service.scheduler.enterImageQueue(r.Context(), modelID, imageWorkHint(r, body).Work, requestIsBorrowed(r))
	if queueErr != nil {
		acquired.release()
		service.writeClientError(w, http.StatusBadGateway, "backend_error", queueErr)
		return nil, true
	}
	switch admission.outcome {
	case offloadReturned:
		acquired.release()
		writeOffloadReturned(w)
		return nil, true
	case offloadWithdrawn:
		if service.forwardOffloadedImageRequest(w, r, request, requestBody, admission.entry, acquired) {
			return nil, true
		}
		requeued := service.scheduler.imageQueue.Requeue(modelID, imageWorkHint(r, body).Work, 0, admission.entry.arrived)
		if outcome, waitErr := service.scheduler.imageQueue.Await(r.Context(), requeued); waitErr != nil || outcome != offloadAdmitted {
			acquired.release()
			openai.WriteError(w, http.StatusBadGateway, "backend_error", "returned request could not be re-queued")
			return nil, true
		}
		return func() { service.scheduler.completeImageQueueEntry(modelID, requeued) }, false
	default:
		return func() { service.scheduler.completeImageQueueEntry(modelID, admission.entry) }, false
	}
}

func (service *Service) handleRegistryImageOptions(w http.ResponseWriter, r *http.Request, body []byte, publicImageID string) bool {
	activeConfigFilename := service.imageCatalogConfigSelector()
	model, hasImageModel := service.registry.ImageModel(publicImageID, activeConfigFilename)
	if !hasImageModel {
		return false
	}
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return true
	}
	acquired, ok := service.acquireRegistryImageRoute(w, r, publicImageID, model, modelBackendMode, activeConfigFilename)
	if !ok {
		return true
	}
	if acquired.route.Remote {
		service.forwardRemoteImageOptions(w, r, body, acquired)
		return true
	}
	service.loadLocalImageOptions(w, r, acquired)
	return true
}

func (service *Service) forwardRemoteImageOptions(w http.ResponseWriter, r *http.Request, body []byte, acquired acquiredRegistryRoute) {
	route := acquired.route
	request := rewriteImageRequest(r, acquired.publicID, route.LocalImageID)
	requestBody := rewriteImageRequestBody(body, acquired.publicID, route.LocalImageID, r)
	response, err := service.forwardRemote(r.Context(), request, requestBody, route)
	if err != nil {
		acquired.release()
		service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
		return
	}
	response = responseWithRelease(response, acquired.release)
	_ = service.writeProxyResponse(w, response, acquired.publicID, true)
}

func (service *Service) loadLocalImageOptions(w http.ResponseWriter, r *http.Request, acquired acquiredRegistryRoute) {
	route := acquired.route
	modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancelModelContext()
	routeBackendMode, err := service.clusterRouteBackendMode(route, acquired.model)
	if err != nil {
		acquired.release()
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if routeBackendMode == BackendModeLlamaSDCPP && clusterModelNeedsPrimaryTextRuntime(acquired.model) {
		if err := service.loadLocalConfig(modelContext, routeBackendMode, route.PublicID, route.Filename, readinessText); err != nil {
			acquired.release()
			service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
			return
		}
	}
	_, releaseModel, _, err := service.acquireModelConfigForBackendMode(routeBackendMode, modelContext, route.PublicImageID, route.Filename, readinessImage, false)
	if err != nil {
		acquired.release()
		service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
		return
	}
	releaseModel()
	acquired.release()
	openai.WriteJSON(w, http.StatusOK, map[string]any{})
}
