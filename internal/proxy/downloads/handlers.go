package downloads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/hardware"
	"tensors-router/internal/openai"
	"tensors-router/internal/proxy/clusterfan"
	"tensors-router/internal/siteapi"
)

func (handlers *Handlers) SiteCapabilities(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	response := siteapi.DownloadCapabilitiesResponse{Nodes: []siteapi.DownloadCapability{handlers.localDownloadCapability(r.Context())}}
	if handlers.deps.ClusterRole() == cluster.RoleMaster {
		for _, result := range clusterfan.Nodes(r.Context(), handlers.deps.RemoteInventoryURLs(), func(ctx context.Context, nodeURL string) (siteapi.DownloadCapability, error) {
			var capability siteapi.DownloadCapability
			err := handlers.deps.ClusterClient().JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/site/download/capabilities", nil, &capability)
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

func (handlers *Handlers) NodeCapabilities(w http.ResponseWriter, r *http.Request) {
	openai.WriteJSON(w, http.StatusOK, handlers.localDownloadCapability(r.Context()))
}

func (handlers *Handlers) localDownloadCapability(ctx context.Context) siteapi.DownloadCapability {
	capability := handlers.capability
	if handlers.downloader != nil {
		capability = downloader.MergeRuntimeCapability(capability, handlers.downloader.Capability())
	}
	devices := []downloader.DeviceCapability{}
	info := handlers.deps.Hardware().Info(ctx)
	for index, device := range info.Devices {
		deviceID := device.DeviceID
		if deviceID == "" {
			deviceID = fmt.Sprintf("%d", index)
		}
		devices = append(devices, downloader.DeviceCapability{Backend: info.GPUBackend, DeviceID: deviceID, Name: device.Name, TotalVRAMBytes: device.TotalVRAMBytes, Architecture: device.Architecture, BackendVersion: firstHardwareVersion(info), SplitOffloadSupported: handlers.deps.BackendMode() == backendmode.LlamaSDCPP && len(info.Devices) > 1})
	}
	for index := len(info.Devices); index < info.GPUCount; index++ {
		devices = append(devices, downloader.DeviceCapability{Backend: info.GPUBackend, DeviceID: fmt.Sprintf("%d", index), Name: fmt.Sprintf("GPU %d", index), BackendVersion: firstHardwareVersion(info), SplitOffloadSupported: false})
	}
	return siteapi.DownloadCapability{NodeID: handlers.deps.NodeID(), NodeURL: handlers.deps.NodeURL(), Available: capability.Available, Capability: capability, Devices: devices}
}

func firstHardwareVersion(info hardware.Info) string {
	if info.CUDAVersion != "" {
		return info.CUDAVersion
	}
	return info.ROCmVersion
}

func (handlers *Handlers) SiteSearch(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if results, err := handlers.downloadSearch(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, results)
	}
}

func (handlers *Handlers) NodeSearch(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	results, err := handlers.downloader.Search(r.Context(), request.SearchRequest, request.Token)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, results)
}

func (handlers *Handlers) SiteSearchPage(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	page, err := handlers.downloadSearchPage(r.Context(), request)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, page)
}

func (handlers *Handlers) NodeSearchPage(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadSearchRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	page, err := handlers.downloader.SearchPage(r.Context(), request.SearchRequest, request.Token)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, page)
}

func (handlers *Handlers) SiteRepository(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadRepositoryRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if response, err := handlers.downloadRepository(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (handlers *Handlers) NodeRepository(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadRepositoryRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	response, err := handlers.downloader.Repository(r.Context(), request.RepositoryRequest)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (handlers *Handlers) SitePlan(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadPlanRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if response, err := handlers.downloadPlan(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (handlers *Handlers) NodePlan(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadPlanRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	response, err := handlers.downloader.Plan(r.Context(), request.PlanRequest)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (handlers *Handlers) SiteCreateJob(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.DownloadCreateJobRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if response, err := handlers.downloadCreateJob(r.Context(), request); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusAccepted, response)
	}
}

func (handlers *Handlers) NodeCreateJob(w http.ResponseWriter, r *http.Request) {
	var request siteapi.DownloadCreateJobRequest
	if !decodeDownloadRequest(w, r, &request) {
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	response, err := handlers.downloader.CreateJob(r.Context(), request.CreateJobRequest)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusAccepted, response)
}

func (handlers *Handlers) SiteJob(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	jobID, ok := downloadJobID(r.URL.Path, "", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := handlers.downloadJob(r.Context(), r.URL.Query().Get("node_id"), jobID); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (handlers *Handlers) NodeJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := downloadJobID(r.URL.Path, "", "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	response, found, err := handlers.downloader.Job(jobID)
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

func (handlers *Handlers) SitePause(w http.ResponseWriter, r *http.Request) {
	handlers.siteJobAction(w, r, "pause")
}
func (handlers *Handlers) SiteResume(w http.ResponseWriter, r *http.Request) {
	handlers.siteJobAction(w, r, "resume")
}
func (handlers *Handlers) SiteCancel(w http.ResponseWriter, r *http.Request) {
	handlers.siteJobAction(w, r, "cancel")
}
func (handlers *Handlers) NodePause(w http.ResponseWriter, r *http.Request) {
	handlers.nodeJobAction(w, r, "pause")
}
func (handlers *Handlers) NodeResume(w http.ResponseWriter, r *http.Request) {
	handlers.nodeJobAction(w, r, "resume")
}
func (handlers *Handlers) NodeCancel(w http.ResponseWriter, r *http.Request) {
	handlers.nodeJobAction(w, r, "cancel")
}

func (handlers *Handlers) siteJobAction(w http.ResponseWriter, r *http.Request, action string) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	jobID, ok := downloadJobID(r.URL.Path, "/"+action, "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := handlers.downloadJobAction(r.Context(), r.URL.Query().Get("node_id"), jobID, action); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (handlers *Handlers) nodeJobAction(w http.ResponseWriter, r *http.Request, action string) {
	jobID, ok := downloadJobID(r.URL.Path, "/"+action, "")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	var response downloader.DownloadJob
	var err error
	switch action {
	case "pause":
		response, err = handlers.downloader.Pause(jobID)
	case "resume":
		response, err = handlers.downloader.Resume(jobID)
	case "cancel":
		response, err = handlers.downloader.Cancel(jobID)
	}
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (handlers *Handlers) SiteLibrary(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := handlers.downloadLibrary(r.Context(), r.URL.Query().Get("node_id")); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (handlers *Handlers) NodeLibrary(w http.ResponseWriter, r *http.Request) {
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	artifacts, err := handlers.downloader.Artifacts()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	jobs, err := handlers.downloader.Jobs()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, siteapi.DownloadLibraryResponse{Artifacts: artifacts, Jobs: jobs})
}

func (handlers *Handlers) SiteRescan(w http.ResponseWriter, r *http.Request) {
	if !handlers.deps.SiteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if response, err := handlers.downloadRescan(r.Context(), r.URL.Query().Get("node_id")); err != nil {
		writeDownloadError(w, err)
		return
	} else {
		openai.WriteJSON(w, http.StatusOK, response)
	}
}

func (handlers *Handlers) NodeRescan(w http.ResponseWriter, r *http.Request) {
	if handlers.downloader == nil {
		writeDownloadUnavailable(w, handlers.capability)
		return
	}
	artifacts, err := handlers.downloader.Rescan()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
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

func writeDownloadUnavailable(w http.ResponseWriter, capability downloader.Capability) {
	openai.WriteError(w, http.StatusServiceUnavailable, "download_unavailable", unavailableError(capability).Error())
}
func writeDownloadError(w http.ResponseWriter, err error) {
	openai.WriteError(w, http.StatusBadRequest, "download_error", err.Error())
}
