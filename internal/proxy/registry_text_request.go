package proxy

import (
	"fmt"
	"net/http"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

type acquiredRegistryRoute struct {
	publicID string
	model    cluster.Model
	route    cluster.Route
	release  func()
}

type registryTextPlan struct {
	modelBackendMode string
	backendModelID   string
	readiness        backendReadiness
	profile          catalog.ChatTemplateProfile
	requestBody      []byte
	usageInjected    bool
}

type registryForward struct {
	response *http.Response
	tracking forwardAnalytics
	complete func()
}

func (service *Service) handleRegistryModelRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string) {
	hint := service.scheduler.textWorkHint(service.nodeID, publicID, int64(len(body)), body)
	model, route, release, ok := service.acquireRegistryModelRoute(r, publicID)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", publicID))
		return
	}
	if borrowedRequestMustStayOnThisNode(r, route) {
		release()
		writeOffloadReturned(w)
		return
	}
	acquired := acquiredRegistryRoute{publicID: publicID, model: model, route: route, release: release}
	service.handleAcquiredRegistryModelRequest(w, r, body, acquired, false, hint)
}

func (service *Service) handleAcquiredRegistryModelRequest(w http.ResponseWriter, r *http.Request, body []byte, acquired acquiredRegistryRoute, insertModel bool, workHint requestWorkHint) {
	defer acquired.release()
	plan, ok := service.prepareRegistryTextRequest(w, r, body, acquired, insertModel)
	if !ok {
		return
	}
	forwarded, ok := service.forwardRegistryTextRequest(w, r, body, acquired, plan, workHint)
	defer forwarded.complete()
	if !ok {
		return
	}
	service.writeRegistryTextResponse(w, r, forwarded, acquired, plan)
}

func (service *Service) prepareRegistryTextRequest(w http.ResponseWriter, r *http.Request, body []byte, acquired acquiredRegistryRoute, insertModel bool) (registryTextPlan, bool) {
	model, route := acquired.model, acquired.route
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return registryTextPlan{}, false
	}
	if modelBackendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteEndpointNotFound(w)
		return registryTextPlan{}, false
	}
	if !registryModelSupportsOpenAIPath(model, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", acquired.publicID))
		return registryTextPlan{}, false
	}
	plan := registryTextPlan{
		modelBackendMode: modelBackendMode,
		backendModelID:   route.LocalID,
		readiness:        modelReadiness(r.URL.Path),
		profile:          service.localChatTemplateProfile(route.Filename, route.Remote),
	}
	if modelBackendMode == BackendModeVLLM {
		plan.backendModelID = vllmRequestModelID(acquired.publicID, route.LocalID, model.ServedNames)
		plan.readiness = vllmReadinessForTask(r.URL.Path, model.VLLMTask)
	}
	plan.requestBody, err = transformBufferedTransportRequestBody(r, body, plan.backendModelID, plan.readiness, plan.profile, true, insertModel)
	if err != nil {
		writeTransportError(w, err)
		return registryTextPlan{}, false
	}
	if !route.Remote {
		plan.requestBody, plan.usageInjected = injectStreamUsageOption(plan.requestBody, r.URL.Path, modelBackendMode)
	}
	return plan, true
}

func (service *Service) forwardRegistryTextRequest(w http.ResponseWriter, r *http.Request, body []byte, acquired acquiredRegistryRoute, plan registryTextPlan, workHint requestWorkHint) (registryForward, bool) {
	forwarded := registryForward{complete: func() {}}
	if acquired.route.Remote {
		response, err := service.forwardRemote(r.Context(), r, plan.requestBody, acquired.route)
		if err != nil {
			service.writeBackendFailure(w, err)
			return forwarded, false
		}
		forwarded.response = response
		return forwarded, true
	}
	routeBackendMode, ok := service.admitLocalRegistryTextRequest(w, r, acquired, plan)
	if !ok {
		return forwarded, false
	}
	if !isEmbeddingsPath(r.URL.Path) {
		complete, handled := service.serveTextThroughLendingQueue(w, r, body, plan.requestBody, acquired, workHint)
		if handled {
			return forwarded, false
		}
		if complete != nil {
			forwarded.complete = complete
		}
	}
	var err error
	forwarded.response, forwarded.tracking, err = service.forwardLocalRegistryTextRequest(r, body, acquired, plan, routeBackendMode)
	if err != nil {
		forwarded.tracking.recordFailure(r.Context(), err)
		service.writeBackendFailure(w, err)
		return forwarded, false
	}
	return forwarded, true
}

func (service *Service) admitLocalRegistryTextRequest(w http.ResponseWriter, r *http.Request, acquired acquiredRegistryRoute, plan registryTextPlan) (string, bool) {
	route := acquired.route
	routeBackendMode, err := service.clusterRouteBackendMode(route, acquired.model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return "", false
	}
	if service.borrowedRequestWouldLoadWithoutPermission(r, routeBackendMode, plan.readiness, route.Filename) {
		writeOffloadReturned(w)
		return "", false
	}
	if routeBackendMode == BackendModeLlamaSDCPP && acquired.model.HasImage && !isEmbeddingsPath(r.URL.Path) {
		if err := service.loadLocalRuntimeForRequest(r.Context(), routeBackendMode, route.PublicImageID, route.Filename, readinessImage); err != nil {
			service.writeBackendFailure(w, err)
			return "", false
		}
	}
	return routeBackendMode, true
}

func (service *Service) forwardLocalRegistryTextRequest(r *http.Request, body []byte, acquired acquiredRegistryRoute, plan registryTextPlan, routeBackendMode string) (*http.Response, forwardAnalytics, error) {
	tracking := forwardAnalytics{analytics: service.analytics}
	tracking.event = service.analytics.newEvent(time.Now(), r, plan.requestBody, plan.backendModelID, textAnalyticsSection(r.URL.Path), routeBackendMode)
	tracking.event.PromptBytes = int64(len(body))
	forwardModelID := acquired.route.PublicID
	if routeBackendMode == BackendModeVLLM {
		forwardModelID = plan.backendModelID
	}
	target := backendForwardTarget{modelID: forwardModelID, configFilename: acquired.route.Filename, hasModel: true, readiness: plan.readiness, mode: routeBackendMode}
	response, finalizer, err := service.forwardWithFallbackObserved(r.Context(), r, plan.requestBody, target)
	tracking.finalizer = finalizer
	return response, tracking, err
}

func (service *Service) writeRegistryTextResponse(w http.ResponseWriter, r *http.Request, forwarded registryForward, acquired acquiredRegistryRoute, plan registryTextPlan) {
	response := forwarded.response
	route := acquired.route
	if plan.modelBackendMode == BackendModeVLLM && r.URL.Path == "/v1/responses" {
		response = service.responseWithVLLMTracking(response, vllmResponseTarget{
			publicID:       acquired.publicID,
			localID:        plan.backendModelID,
			configFilename: route.Filename,
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
		})
	}
	response = responseWithRelease(response, acquired.release)
	response = forwarded.tracking.wrap(response)
	response = clientStreamUsage(response, plan.usageInjected)
	_ = service.writeModelProxyResponse(w, response, acquired.publicID, true)
}

func (service *Service) serveTextThroughLendingQueue(w http.ResponseWriter, r *http.Request, body []byte, requestBody []byte, acquired acquiredRegistryRoute, workHint requestWorkHint) (complete func(), handled bool) {
	modelID := acquired.route.LocalID
	if !service.scheduler.queuesForLending(cluster.RouteLaneText, acquired.route.NodeID, modelID) {
		return nil, false
	}
	admission, queueErr := service.scheduler.enterTextQueue(r.Context(), modelID, workHint.Work, int64(workHint.RequiredContext), requestIsStreaming(body), requestIsBorrowed(r))
	if queueErr != nil {
		service.writeClientError(w, http.StatusBadGateway, "backend_error", queueErr)
		return nil, true
	}
	switch admission.outcome {
	case offloadReturned:
		writeOffloadReturned(w)
		return nil, true
	case offloadWithdrawn:
		return service.serveWithdrawnTextRequest(w, r, requestBody, acquired, admission.entry, workHint)
	default:
		return func() { service.scheduler.completeTextQueueEntry(modelID, admission.entry) }, false
	}
}

func (service *Service) serveWithdrawnTextRequest(w http.ResponseWriter, r *http.Request, requestBody []byte, acquired acquiredRegistryRoute, entry *offloadEntry, workHint requestWorkHint) (complete func(), handled bool) {
	modelID := acquired.route.LocalID
	if !service.lentTextRequestFitsHelper(modelID, entry) {
		service.scheduler.finishOffload(cluster.RouteLaneText, modelID, entry, true)
	} else if service.forwardOffloadedTextRequest(w, r, r, requestBody, entry, acquired) {
		return nil, true
	}
	requeued := service.scheduler.textQueue.Requeue(modelID, workHint.Work, int64(workHint.RequiredContext), entry.arrived)
	if outcome, waitErr := service.scheduler.textQueue.Await(r.Context(), requeued); waitErr != nil || outcome != offloadAdmitted {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", "returned request could not be re-queued")
		return nil, true
	}
	return func() { service.scheduler.completeTextQueueEntry(modelID, requeued) }, false
}
