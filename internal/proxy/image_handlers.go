package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

type imageModelObject struct {
	Title     string `json:"title"`
	ModelName string `json:"model_name"`
	Hash      string `json:"hash"`
	SHA256    string `json:"sha256"`
	Filename  string `json:"filename"`
	Config    string `json:"config"`
}

func (service *Service) handleImageModels(w http.ResponseWriter) {
	if service.registry != nil {
		openai.WriteJSON(w, http.StatusOK, clusterImageModelObjects(service.registry.Models(), service.imageCatalogConfigSelector()))
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, clusterImageModelObjects(cluster.LocalModelsWithBackendMode(models, service.nodeID, service.nodeURL, service.localSource(), service.backendMode), service.imageCatalogConfigSelector()))
}

func (service *Service) handleImageOptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		model, err := service.activeImageModel(r)
		if err != nil {
			if lookupErr, ok := err.(imageModelLookupError); ok && lookupErr.status == http.StatusBadRequest {
				openai.WriteJSON(w, http.StatusOK, map[string]any{
					"sd_model_checkpoint": "",
				})
				return
			}
			openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
			return
		}
		openai.WriteJSON(w, http.StatusOK, map[string]any{
			"sd_model_checkpoint": model.ImageID,
		})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
			return
		}
		defer r.Body.Close()

		modelID, hasModel, err := imageModelFromRequest(body, r)
		if err != nil {
			service.logger.Printf("image model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if hasModel {
			if service.handleRecipeImageRequest(w, r, body, modelID) {
				return
			}
			if service.registry != nil && service.handleRegistryImageOptions(w, r, body, modelID) {
				return
			}
			model, err := service.resolveImageModel(r, modelID)
			if err != nil {
				writeImageModelError(service, w, r, modelID, err)
				return
			}
			modelBackendMode, err := service.catalogModelBackendMode(model)
			if err != nil {
				openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			if modelBackendMode == BackendModeLlamaSDCPP && modelNeedsPrimaryTextRuntime(model) {
				if err := service.loadLocalRuntimeForRequest(r.Context(), modelBackendMode, model.ID, model.Filename, readinessText); err != nil {
					openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
					return
				}
			}
			modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
			defer cancelModelContext()
			_, release, _, err := service.acquireModelConfigForBackendMode(modelBackendMode, modelContext, model.ImageID, model.Filename, readinessImage, false)
			if err != nil {
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return
			}
			release()
		}
		openai.WriteJSON(w, http.StatusOK, map[string]any{})
	default:
		openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (service *Service) handleImageRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
		return
	}
	defer r.Body.Close()

	modelID, hasModel, err := imageModelFromRequest(body, r)
	if err != nil {
		service.logger.Printf("image model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	if !hasModel {
		if target, ok := service.sdcppJobs.routeForPath(r.URL.Path); ok {
			if target.remote {
				response, err := service.forwardRemote(r.Context(), r, body, cluster.Route{NodeURL: target.nodeURL, Remote: true})
				if err != nil {
					openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
					return
				}
				if err := service.writeProxyResponse(w, response, target.publicImageID, false); err != nil {
					return
				}
				return
			}
			started := time.Now()
			analyticsEvent := service.newAnalyticsEvent(started, r, body, target.publicImageID, routeranalytics.SectionImage, target.backendMode)
			response, workFinalizer, err := service.forwardWithFallbackObserved(r.Context(), r, body, target.publicImageID, target.configFilename, true, readinessImage, target.backendMode)
			if err != nil {
				service.recordAnalyticsFailure(analyticsEvent, http.StatusBadGateway, workFinalizer)
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return
			}
			response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
			if err := service.writeProxyResponse(w, response, target.publicImageID, false); err != nil {
				return
			}
			return
		}
		if isImageDiscoveryPath(r.URL.Path) {
			selectedBackendMode, err := service.resolveBackendMode("")
			if err != nil {
				openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			response, err := service.forwardWithFallback(r.Context(), r, body, "", "", false, readinessImage, selectedBackendMode)
			if err != nil {
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return
			}
			if err := service.writeProxyResponse(w, response, "", false); err != nil {
				return
			}
			return
		}
	}

	var model catalog.Model
	if hasModel {
		if service.handleRecipeImageRequest(w, r, body, modelID) {
			return
		}
		if service.registry != nil && service.handleRegistryImageRequest(w, r, body, modelID) {
			return
		}
		model, err = service.resolveImageModel(r, modelID)
		if err != nil {
			writeImageModelError(service, w, r, modelID, err)
			return
		}
	} else {
		model, err = service.activeImageModel(r)
		if err != nil {
			writeImageModelError(service, w, r, modelID, err)
			return
		}
		hasModel = true
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	if modelBackendMode == BackendModeLlamaSDCPP && modelNeedsPrimaryTextRuntime(model) {
		if err := service.loadLocalRuntimeForRequest(r.Context(), modelBackendMode, model.ID, model.Filename, readinessText); err != nil {
			openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
			return
		}
	}

	started := time.Now()
	analyticsEvent := service.newAnalyticsEvent(started, r, body, model.ImageID, routeranalytics.SectionImage, modelBackendMode)
	response, workFinalizer, err := service.forwardWithFallbackObserved(r.Context(), r, body, model.ImageID, model.Filename, hasModel, readinessImage, modelBackendMode)
	if err != nil {
		status, _, _ := backendFailureResponse(err)
		service.recordAnalyticsFailure(analyticsEvent, status, workFinalizer)
		writeBackendFailure(w, err)
		return
	}

	if isSdcppJobSubmissionPath(r.URL.Path) {
		response = service.responseWithSdcppJobTracking(response, sdcppJobTarget{
			publicImageID:  model.ImageID,
			configFilename: model.Filename,
			backendMode:    modelBackendMode,
		})
	}
	response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)

	if err := service.writeProxyResponse(w, response, model.ImageID, hasModel); err != nil {
		return
	}
}

type imageModelLookupError struct {
	status  int
	message string
}

func (err imageModelLookupError) Error() string {
	return err.message
}

func (service *Service) resolveImageModel(r *http.Request, modelID string) (catalog.Model, error) {
	model, ok, err := service.resolveVisibleImageModel(modelID)
	if err != nil {
		service.logger.Printf("image model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
		return catalog.Model{}, err
	}
	if !ok {
		service.logger.Printf("unknown image model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		return catalog.Model{}, imageModelLookupError{
			status:  http.StatusNotFound,
			message: fmt.Sprintf("image model %q was not found", modelID),
		}
	}
	return model, nil
}

func (service *Service) activeImageModel(r *http.Request) (catalog.Model, error) {
	model, ok, err := service.resolveActiveImageModel()
	if err != nil {
		service.logger.Printf("active image model catalog check failed path=%s error=%v", r.URL.Path, err)
		return catalog.Model{}, err
	}
	if !ok {
		service.logger.Printf("image model missing path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		return catalog.Model{}, imageModelLookupError{
			status:  http.StatusBadRequest,
			message: "image model is required",
		}
	}
	return model, nil
}

func (service *Service) resolveVisibleImageModel(modelID string) (catalog.Model, bool, error) {
	modelID = strings.TrimSpace(modelID)
	models, err := service.catalog.List()
	if err != nil {
		return catalog.Model{}, false, err
	}
	activeConfigFilename := service.currentImageConfigFilename()
	for _, model := range models {
		if strings.TrimSpace(model.ImageID) != modelID {
			continue
		}
		visible, err := service.catalogImageModelVisible(model, activeConfigFilename)
		if err != nil {
			return catalog.Model{}, false, err
		}
		if visible {
			return model, true, nil
		}
	}
	return catalog.Model{}, false, nil
}

func (service *Service) resolveActiveImageModel() (catalog.Model, bool, error) {
	activeConfigFilename := service.currentImageConfigFilename()
	if strings.TrimSpace(activeConfigFilename) == "" {
		return catalog.Model{}, false, nil
	}
	models, err := service.catalog.List()
	if err != nil {
		return catalog.Model{}, false, err
	}
	activeBackendMode := service.currentBackendMode()
	for _, model := range models {
		if !model.HasImage || strings.TrimSpace(model.ImageID) == "" || model.Filename != activeConfigFilename {
			continue
		}
		modelBackendMode, err := service.catalogModelBackendMode(model)
		if err != nil {
			return catalog.Model{}, false, err
		}
		if modelBackendMode == activeBackendMode {
			return model, true, nil
		}
	}
	return catalog.Model{}, false, nil
}

func (service *Service) catalogImageModelVisible(model catalog.Model, activeConfigFilename string) (bool, error) {
	if !model.HasImage || strings.TrimSpace(model.ImageID) == "" {
		return false, nil
	}
	if !model.HasLLM {
		return true, nil
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return false, err
	}
	if modelBackendMode == BackendModeLlamaSDCPP {
		return true, nil
	}
	return model.Filename == activeConfigFilename, nil
}

func writeImageModelError(service *Service, w http.ResponseWriter, r *http.Request, modelID string, err error) {
	if lookupErr, ok := err.(imageModelLookupError); ok {
		openai.WriteError(w, lookupErr.status, "invalid_request_error", lookupErr.message)
		return
	}
	service.logger.Printf("image model lookup failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
	openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
}
