package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteModelAssetCreateJob(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetConfigRequest(w, r)
	if !ok {
		return
	}
	target, err := service.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid config node")
		return
	}
	request.NodeID, request.NodeURL = target.nodeID, target.nodeURL
	if target.local {
		job, err := service.createLocalModelAssetJob(request)
		if err != nil {
			openai.WriteError(w, http.StatusBadRequest, "model_asset_error", "resolution job could not be created")
			return
		}
		openai.WriteJSON(w, http.StatusAccepted, job)
		return
	}
	var job modelassets.ResolutionJob
	if err := service.clusterClient.JSON(r.Context(), http.MethodPost, target.nodeURL, "/router/v1/node/site/model-assets/jobs", request, &job); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", "resolution job could not be created")
		return
	}
	openai.WriteJSON(w, http.StatusAccepted, job)
}

func (service *Service) handleNodeModelAssetCreateJob(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetConfigRequest(w, r)
	if !ok {
		return
	}
	job, err := service.createLocalModelAssetJob(request)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "model_asset_error", "resolution job could not be created")
		return
	}
	openai.WriteJSON(w, http.StatusAccepted, job)
}

func (service *Service) createLocalModelAssetJob(request siteapi.ModelAssetConfigRequest) (modelassets.ResolutionJob, error) {
	if service.assetIndex == nil {
		return modelassets.ResolutionJob{}, fmt.Errorf("model asset index is unavailable")
	}
	_, _, target, err := service.modelAssetConfigTarget(request)
	if err != nil {
		return modelassets.ResolutionJob{}, err
	}
	job, _, err := service.startSharedLocalModelAssetJob(request, target)
	return job, err
}

func (service *Service) startSharedLocalModelAssetJob(request siteapi.ModelAssetConfigRequest, target string) (modelassets.ResolutionJob, *activeConfigResolution, error) {
	candidate := &activeConfigResolution{ready: make(chan struct{}), done: make(chan struct{})}
	value, loaded := service.assetResolutionJobs.LoadOrStore(target, candidate)
	active := value.(*activeConfigResolution)
	if loaded {
		<-active.ready
		return active.job, active, active.err
	}
	id, _, _, err := service.modelAssetConfigTarget(request)
	if err == nil {
		active.job, err = service.assetIndex.CreateResolutionJob(id, service.nodeID)
	}
	active.err = err
	close(active.ready)
	if err != nil {
		service.assetResolutionJobs.Delete(target)
		close(active.done)
		return modelassets.ResolutionJob{}, active, err
	}
	go func() {
		defer service.assetResolutionJobs.Delete(target)
		defer close(active.done)
		service.runLocalModelAssetJob(request, active.job)
	}()
	return active.job, active, nil
}

func (service *Service) runLocalModelAssetJob(request siteapi.ModelAssetConfigRequest, job modelassets.ResolutionJob) {
	job.State = modelassets.JobResolving
	job.Source = "automatic"
	_ = service.assetIndex.UpdateResolutionJob(job)
	_ = service.refreshLocalRegistry()
	response, err := service.resolveLocalModelAssetConfig(request)
	job.Results = make([]modelassets.FieldResult, len(response.Results))
	for index, result := range response.Results {
		job.Results[index] = modelassets.FieldResult{Field: result.Field, Hash: result.Hash, Resolved: result.Resolved, Failure: result.Failure, Source: result.Source, Verification: result.Verification, Commit: result.Commit}
		if result.Source != "" {
			job.Source = result.Source
		}
		if result.Resolved {
			if asset, found := service.assetIndex.Lookup(result.Hash); found {
				job.ProgressBytes += asset.Size
				job.TotalBytes += asset.Size
			}
		}
	}
	if err != nil {
		job.State = modelassets.JobFailed
		job.Error = "resolution failed"
	} else {
		job.State = modelassets.JobCompleted
		for _, result := range response.Results {
			if !result.Resolved {
				job.State = modelassets.JobFailed
				job.Error = "model asset unavailable"
				break
			}
		}
	}
	if updateErr := service.assetIndex.UpdateResolutionJob(job); updateErr != nil {
		service.logger.Printf("model asset job persistence failed job=%s error_type=%T", job.ID, updateErr)
	}
	_ = service.refreshLocalRegistry()
}

func (service *Service) handleSiteModelAssetJob(w http.ResponseWriter, r *http.Request) {
	jobID, events, ok := modelAssetJobPath(r.URL.Path, "/router/v1/site/model-assets/jobs/")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	target, err := service.configNodeTarget(r.URL.Query().Get("node_id"), "")
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid config node")
		return
	}
	if !target.local {
		path := "/router/v1/node/site/model-assets/jobs/" + jobID
		if events {
			path += "/events"
			service.streamRemoteModelAssetJob(w, r, target.nodeURL, path)
			return
		}
		var job modelassets.ResolutionJob
		if err := service.clusterClient.JSON(r.Context(), http.MethodGet, target.nodeURL, path, nil, &job); err != nil {
			openai.WriteError(w, http.StatusBadGateway, "cluster_error", "resolution job unavailable")
			return
		}
		openai.WriteJSON(w, http.StatusOK, job)
		return
	}
	if events {
		service.streamLocalModelAssetJob(w, r, jobID)
		return
	}
	service.writeLocalModelAssetJob(w, jobID)
}

func (service *Service) handleNodeModelAssetJob(w http.ResponseWriter, r *http.Request) {
	jobID, events, ok := modelAssetJobPath(r.URL.Path, "/router/v1/node/site/model-assets/jobs/")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if events {
		service.streamLocalModelAssetJob(w, r, jobID)
		return
	}
	service.writeLocalModelAssetJob(w, jobID)
}

func (service *Service) writeLocalModelAssetJob(w http.ResponseWriter, id string) {
	job, found, err := service.assetIndex.ResolutionJob(id)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "model_asset_error", "resolution job unavailable")
		return
	}
	if !found {
		openai.WriteError(w, http.StatusNotFound, "not_found", "resolution job was not found")
		return
	}
	openai.WriteJSON(w, http.StatusOK, job)
}

func (service *Service) streamLocalModelAssetJob(w http.ResponseWriter, r *http.Request, id string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		openai.WriteError(w, http.StatusInternalServerError, "model_asset_error", "streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, found, err := service.assetIndex.ResolutionJob(id)
		if err != nil || !found {
			return
		}
		content, err := json.Marshal(job)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: progress\ndata: %s\n\n", content)
		flusher.Flush()
		if job.State == modelassets.JobCompleted || job.State == modelassets.JobFailed {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) streamRemoteModelAssetJob(w http.ResponseWriter, r *http.Request, nodeURL string, path string) {
	response, err := service.clusterClient.Stream(r.Context(), http.MethodGet, nodeURL, path)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", "resolution event stream unavailable")
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(webDownloadFlushingWriter{ResponseWriter: w}, response.Body)
}

func modelAssetJobPath(value string, prefix string) (string, bool, bool) {
	value = strings.TrimPrefix(value, prefix)
	events := strings.HasSuffix(value, "/events")
	value = strings.TrimSuffix(value, "/events")
	if len(value) != 32 {
		return "", false, false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return "", false, false
		}
	}
	return value, events, true
}
