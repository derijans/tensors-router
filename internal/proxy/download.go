package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/hardware"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteDownloadCapabilities(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	response := siteapi.DownloadCapabilitiesResponse{Nodes: []siteapi.DownloadCapability{service.localDownloadCapability(r.Context())}}
	if service.clusterRole == cluster.RoleMaster {
		for _, result := range fanOutNodes(r.Context(), service.remoteInventoryURLs(), func(ctx context.Context, nodeURL string) (siteapi.DownloadCapability, error) {
			var capability siteapi.DownloadCapability
			err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/site/download/capabilities", nil, &capability)
			return capability, err
		}) {
			if result.Err != nil {
				continue
			}
			response.Nodes = append(response.Nodes, result.Value)
		}
	}
	sort.Slice(response.Nodes, func(left int, right int) bool { return response.Nodes[left].NodeID < response.Nodes[right].NodeID })
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeDownloadCapabilities(w http.ResponseWriter, r *http.Request) {
	openai.WriteJSON(w, http.StatusOK, service.localDownloadCapability(r.Context()))
}

func (service *Service) localDownloadCapability(ctx context.Context) siteapi.DownloadCapability {
	capability := service.downloaderCapability
	if service.downloader != nil {
		capability = downloader.MergeRuntimeCapability(capability, service.downloader.Capability())
	}
	devices := []downloader.DeviceCapability{}
	info := service.hardware.Info(ctx)
	for index, device := range info.Devices {
		deviceID := device.DeviceID
		if deviceID == "" {
			deviceID = fmt.Sprintf("%d", index)
		}
		devices = append(devices, downloader.DeviceCapability{Backend: info.GPUBackend, DeviceID: deviceID, Name: device.Name, TotalVRAMBytes: device.TotalVRAMBytes, Architecture: device.Architecture, BackendVersion: firstHardwareVersion(info), SplitOffloadSupported: service.backendMode == BackendModeLlamaSDCPP && len(info.Devices) > 1})
	}
	for index := len(info.Devices); index < info.GPUCount; index++ {
		devices = append(devices, downloader.DeviceCapability{Backend: info.GPUBackend, DeviceID: fmt.Sprintf("%d", index), Name: fmt.Sprintf("GPU %d", index), BackendVersion: firstHardwareVersion(info), SplitOffloadSupported: false})
	}
	return siteapi.DownloadCapability{NodeID: service.nodeID, NodeURL: service.nodeURL, Available: capability.Available, Capability: capability, Devices: devices}
}

func firstHardwareVersion(info hardware.Info) string {
	if info.CUDAVersion != "" {
		return info.CUDAVersion
	}
	return info.ROCmVersion
}

func (service *Service) handleSiteDownloadSearch(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if results, err := service.downloadSearch(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, results)
	}
}

func (service *Service) handleNodeDownloadSearch(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	results, err := service.downloader.Search(r.Context(), request.SearchRequest, request.Token)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, results)
}

func (service *Service) handleSiteDownloadSearchPage(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	page, err := service.downloadSearchPage(r.Context(), request)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, page)
}

func (service *Service) handleNodeDownloadSearchPage(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	page, err := service.downloader.SearchPage(r.Context(), request.SearchRequest, request.Token)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, page)
}

func (service *Service) handleSiteDownloadRepository(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadRepositoryRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if response, err := service.downloadRepository(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (service *Service) handleNodeDownloadRepository(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadRepositoryRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	response, err := service.downloader.Repository(r.Context(), request.RepositoryRequest)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteDownloadPlan(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadPlanRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if response, err := service.downloadPlan(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (service *Service) handleNodeDownloadPlan(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadPlanRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	response, err := service.downloader.Plan(r.Context(), request.PlanRequest)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteDownloadCreateJob(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadCreateJobRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if response, err := service.downloadCreateJob(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusAccepted, response)
	}
}

func (service *Service) handleNodeDownloadCreateJob(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadCreateJobRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	response, err := service.downloader.CreateJob(r.Context(), request.CreateJobRequest)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusAccepted, response)
}

func (service *Service) handleSiteDownloadJob(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	jobID, ok := downloadJobID(r.URL.Path, "", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := service.downloadJob(r.Context(), r.URL.Query().Get("node_id"), jobID); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (service *Service) handleNodeDownloadJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := downloadJobID(r.URL.Path, "", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	response, found, err := service.downloader.Job(jobID)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	if !found {
		openai.WriteError(w, http.StatusNotFound, "not_found", "download job was not found")
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteDownloadPause(w http.ResponseWriter, r *http.Request) {
	service.handleSiteDownloadJobAction(w, r, "pause")
}
func (service *Service) handleSiteDownloadResume(w http.ResponseWriter, r *http.Request) {
	service.handleSiteDownloadJobAction(w, r, "resume")
}
func (service *Service) handleSiteDownloadCancel(w http.ResponseWriter, r *http.Request) {
	service.handleSiteDownloadJobAction(w, r, "cancel")
}
func (service *Service) handleNodeDownloadPause(w http.ResponseWriter, r *http.Request) {
	service.handleNodeDownloadJobAction(w, r, "pause")
}
func (service *Service) handleNodeDownloadResume(w http.ResponseWriter, r *http.Request) {
	service.handleNodeDownloadJobAction(w, r, "resume")
}
func (service *Service) handleNodeDownloadCancel(w http.ResponseWriter, r *http.Request) {
	service.handleNodeDownloadJobAction(w, r, "cancel")
}

func (service *Service) handleSiteDownloadJobAction(w http.ResponseWriter, r *http.Request, action string) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	jobID, ok := downloadJobID(r.URL.Path, "/"+action, "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := service.downloadJobAction(r.Context(), r.URL.Query().Get("node_id"), jobID, action); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (service *Service) handleNodeDownloadJobAction(w http.ResponseWriter, r *http.Request, action string) {
	jobID, ok := downloadJobID(r.URL.Path, "/"+action, "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	var response downloader.DownloadJob
	var err error
	switch action {
	case "pause":
		response, err = service.downloader.Pause(jobID)
	case "resume":
		response, err = service.downloader.Resume(jobID)
	case "cancel":
		response, err = service.downloader.Cancel(jobID)
	}
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleSiteDownloadLibrary(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := service.downloadLibrary(r.Context(), r.URL.Query().Get("node_id")); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (service *Service) handleNodeDownloadLibrary(w http.ResponseWriter, r *http.Request) {
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	artifacts, err := service.downloader.Artifacts()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	jobs, err := service.downloader.Jobs()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, siteapi.DownloadLibraryResponse{Artifacts: artifacts, Jobs: jobs})
}

func (service *Service) handleSiteDownloadRescan(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := service.downloadRescan(r.Context(), r.URL.Query().Get("node_id")); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (service *Service) handleNodeDownloadRescan(w http.ResponseWriter, r *http.Request) {
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	artifacts, err := service.downloader.Rescan()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}

func (service *Service) handleSiteDownloadEvents(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	jobID, ok := downloadJobID(r.URL.Path, "/events", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if remoteURL, remote, err := service.downloadTarget(r.URL.Query().Get("node_id")); err != nil {
		writeDownloadError(w, err)
		return
	} else if remote {
		service.streamRemoteDownloadEvents(w, r, remoteURL, jobID)
		return
	}
	service.writeDownloadEvents(w, r, jobID)
}

func (service *Service) handleNodeDownloadEvents(w http.ResponseWriter, r *http.Request) {
	jobID, ok := downloadJobID(r.URL.Path, "/events", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	service.writeDownloadEvents(w, r, jobID)
}

func (service *Service) streamRemoteDownloadEvents(w http.ResponseWriter, r *http.Request, nodeURL string, jobID string) {
	response, err := service.clusterClient.Stream(r.Context(), http.MethodGet, nodeURL, "/router/v1/node/site/download/jobs/"+jobID+"/events")
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if response.Header.Get("Content-Encoding") != "" {
		w.Header().Set("Content-Encoding", response.Header.Get("Content-Encoding"))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(webDownloadFlushingWriter{ResponseWriter: w}, response.Body)
}

type webDownloadFlushingWriter struct {
	http.ResponseWriter
}

func (writer webDownloadFlushingWriter) Write(content []byte) (int, error) {
	written, err := writer.ResponseWriter.Write(content)
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return written, err
}

func (service *Service) writeDownloadEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	if service.downloader == nil {
		writeDownloadUnavailable(w, service.downloaderCapability)
		return
	}
	if _, found, err := service.downloader.Job(jobID); err != nil {
		writeDownloadError(w, err)
		return
	} else if !found {
		openai.WriteError(w, http.StatusNotFound, "not_found", "download job was not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		openai.WriteError(w, http.StatusInternalServerError, "download_error", "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	events, unsubscribe := service.downloader.Subscribe(jobID)
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case job := <-events:
			content, err := json.Marshal(job)
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "event: progress\ndata: %s\n\n", content)
			flusher.Flush()
			if job.State == downloader.JobCompleted || job.State == downloader.JobFailed || job.State == downloader.JobCancelled {
				return
			}
		}
	}
}

func (service *Service) downloadSearch(ctx context.Context, request siteapi.DownloadSearchRequest) ([]downloader.SearchResult, error) {
	remoteURL, remote, err := service.downloadTarget(request.NodeID)
	if err != nil {
		return nil, err
	}
	if remote {
		var response []downloader.SearchResult
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/search", request, &response)
		return response, err
	}
	if service.downloader == nil {
		return nil, unavailableError(service.downloaderCapability)
	}
	return service.downloader.Search(ctx, request.SearchRequest, request.Token)
}

func (service *Service) downloadSearchPage(ctx context.Context, request siteapi.DownloadSearchRequest) (downloader.SearchPage, error) {
	remoteURL, remote, err := service.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.SearchPage{}, err
	}
	if remote {
		var response downloader.SearchPage
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/search-page", request, &response)
		return response, err
	}
	if service.downloader == nil {
		return downloader.SearchPage{}, unavailableError(service.downloaderCapability)
	}
	return service.downloader.SearchPage(ctx, request.SearchRequest, request.Token)
}

func (service *Service) downloadRepository(ctx context.Context, request siteapi.DownloadRepositoryRequest) (downloader.RepositoryDetails, error) {
	remoteURL, remote, err := service.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.RepositoryDetails{}, err
	}
	if remote {
		var response downloader.RepositoryDetails
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/repository", request, &response)
		return response, err
	}
	if service.downloader == nil {
		return downloader.RepositoryDetails{}, unavailableError(service.downloaderCapability)
	}
	return service.downloader.Repository(ctx, request.RepositoryRequest)
}

func (service *Service) downloadPlan(ctx context.Context, request siteapi.DownloadPlanRequest) (downloader.DownloadPlan, error) {
	remoteURL, remote, err := service.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.DownloadPlan{}, err
	}
	if remote {
		var response downloader.DownloadPlan
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/plan", request, &response)
		return response, err
	}
	if service.downloader == nil {
		return downloader.DownloadPlan{}, unavailableError(service.downloaderCapability)
	}
	return service.downloader.Plan(ctx, request.PlanRequest)
}

func (service *Service) downloadCreateJob(ctx context.Context, request siteapi.DownloadCreateJobRequest) (downloader.DownloadJob, error) {
	remoteURL, remote, err := service.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if remote {
		var response downloader.DownloadJob
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/jobs", request, &response)
		return response, err
	}
	if service.downloader == nil {
		return downloader.DownloadJob{}, unavailableError(service.downloaderCapability)
	}
	return service.downloader.CreateJob(ctx, request.CreateJobRequest)
}

func (service *Service) downloadJob(ctx context.Context, nodeID string, jobID string) (downloader.DownloadJob, error) {
	remoteURL, remote, err := service.downloadTarget(nodeID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if remote {
		var response downloader.DownloadJob
		err := service.clusterClient.JSON(ctx, http.MethodGet, remoteURL, "/router/v1/node/site/download/jobs/"+jobID, nil, &response)
		return response, err
	}
	if service.downloader == nil {
		return downloader.DownloadJob{}, unavailableError(service.downloaderCapability)
	}
	response, found, err := service.downloader.Job(jobID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if !found {
		return downloader.DownloadJob{}, fmt.Errorf("download job was not found")
	}
	return response, nil
}

func (service *Service) downloadJobAction(ctx context.Context, nodeID string, jobID string, action string) (downloader.DownloadJob, error) {
	remoteURL, remote, err := service.downloadTarget(nodeID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if remote {
		var response downloader.DownloadJob
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/jobs/"+jobID+"/"+action, nil, &response)
		return response, err
	}
	if service.downloader == nil {
		return downloader.DownloadJob{}, unavailableError(service.downloaderCapability)
	}
	switch action {
	case "pause":
		return service.downloader.Pause(jobID)
	case "resume":
		return service.downloader.Resume(jobID)
	case "cancel":
		return service.downloader.Cancel(jobID)
	}
	return downloader.DownloadJob{}, fmt.Errorf("download job action is invalid")
}

func (service *Service) downloadLibrary(ctx context.Context, nodeID string) (siteapi.DownloadLibraryResponse, error) {
	remoteURL, remote, err := service.downloadTarget(nodeID)
	if err != nil {
		return siteapi.DownloadLibraryResponse{}, err
	}
	if remote {
		var response siteapi.DownloadLibraryResponse
		err := service.clusterClient.JSON(ctx, http.MethodGet, remoteURL, "/router/v1/node/site/download/library", nil, &response)
		return response, err
	}
	if service.downloader == nil {
		return siteapi.DownloadLibraryResponse{}, unavailableError(service.downloaderCapability)
	}
	artifacts, err := service.downloader.Artifacts()
	if err != nil {
		return siteapi.DownloadLibraryResponse{}, err
	}
	jobs, err := service.downloader.Jobs()
	if err != nil {
		return siteapi.DownloadLibraryResponse{}, err
	}
	return siteapi.DownloadLibraryResponse{Artifacts: artifacts, Jobs: jobs}, nil
}

func (service *Service) downloadRescan(ctx context.Context, nodeID string) (map[string]any, error) {
	remoteURL, remote, err := service.downloadTarget(nodeID)
	if err != nil {
		return nil, err
	}
	if remote {
		var response map[string]any
		err := service.clusterClient.JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/rescan", nil, &response)
		return response, err
	}
	if service.downloader == nil {
		return nil, unavailableError(service.downloaderCapability)
	}
	artifacts, err := service.downloader.Rescan()
	if err != nil {
		return nil, err
	}
	return map[string]any{"artifacts": artifacts}, nil
}

func (service *Service) downloadTarget(nodeID string) (string, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || nodeID == service.nodeID {
		return "", false, nil
	}
	if service.clusterRole != cluster.RoleMaster {
		return "", false, fmt.Errorf("selected download node is not local")
	}
	nodeURL := strings.TrimSpace(service.nodeURLByID()[nodeID])
	if nodeURL == "" {
		return "", false, fmt.Errorf("download node %q was not found", nodeID)
	}
	return nodeURL, true, nil
}

func decodeDownloadRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return false
	}
	return true
}

func downloadJobID(value string, suffix string, prefix string) (string, bool) {
	value = strings.TrimSuffix(value, suffix)
	value = strings.TrimPrefix(value, "/router/v1/site/download/jobs/")
	value = strings.TrimPrefix(value, "/router/v1/node/site/download/jobs/")
	value = strings.TrimPrefix(value, prefix)
	if len(value) != 32 {
		return "", false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return "", false
		}
	}
	return value, true
}

func unavailableError(capability downloader.Capability) error {
	if capability.Error != "" {
		return fmt.Errorf("downloader is unavailable: %s", capability.Error)
	}
	return fmt.Errorf("downloader is unavailable on this node")
}

func writeDownloadUnavailable(w http.ResponseWriter, capability downloader.Capability) {
	openai.WriteError(w, http.StatusServiceUnavailable, "download_unavailable", unavailableError(capability).Error())
}
func writeDownloadError(w http.ResponseWriter, err error) {
	openai.WriteError(w, http.StatusBadRequest, "download_error", err.Error())
}
