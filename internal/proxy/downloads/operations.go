package downloads

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/siteapi"
)

func (handlers *Handlers) downloadSearch(ctx context.Context, request siteapi.DownloadSearchRequest) ([]downloader.SearchResult, error) {
	remoteURL, remote, err := handlers.downloadTarget(request.NodeID)
	if err != nil {
		return nil, err
	}
	if remote {
		var response []downloader.SearchResult
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/search", request, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return nil, unavailableError(handlers.capability)
	}
	return handlers.downloader.Search(ctx, request.SearchRequest, request.Token)
}

func (handlers *Handlers) downloadSearchPage(ctx context.Context, request siteapi.DownloadSearchRequest) (downloader.SearchPage, error) {
	remoteURL, remote, err := handlers.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.SearchPage{}, err
	}
	if remote {
		var response downloader.SearchPage
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/search-page", request, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return downloader.SearchPage{}, unavailableError(handlers.capability)
	}
	return handlers.downloader.SearchPage(ctx, request.SearchRequest, request.Token)
}

func (handlers *Handlers) downloadRepository(ctx context.Context, request siteapi.DownloadRepositoryRequest) (downloader.RepositoryDetails, error) {
	remoteURL, remote, err := handlers.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.RepositoryDetails{}, err
	}
	if remote {
		var response downloader.RepositoryDetails
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/repository", request, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return downloader.RepositoryDetails{}, unavailableError(handlers.capability)
	}
	return handlers.downloader.Repository(ctx, request.RepositoryRequest)
}

func (handlers *Handlers) downloadPlan(ctx context.Context, request siteapi.DownloadPlanRequest) (downloader.DownloadPlan, error) {
	remoteURL, remote, err := handlers.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.DownloadPlan{}, err
	}
	if remote {
		var response downloader.DownloadPlan
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/plan", request, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return downloader.DownloadPlan{}, unavailableError(handlers.capability)
	}
	return handlers.downloader.Plan(ctx, request.PlanRequest)
}

func (handlers *Handlers) downloadCreateJob(ctx context.Context, request siteapi.DownloadCreateJobRequest) (downloader.DownloadJob, error) {
	remoteURL, remote, err := handlers.downloadTarget(request.NodeID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if remote {
		var response downloader.DownloadJob
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/jobs", request, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return downloader.DownloadJob{}, unavailableError(handlers.capability)
	}
	return handlers.downloader.CreateJob(ctx, request.CreateJobRequest)
}

func (handlers *Handlers) downloadJob(ctx context.Context, nodeID string, jobID string) (downloader.DownloadJob, error) {
	remoteURL, remote, err := handlers.downloadTarget(nodeID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if remote {
		var response downloader.DownloadJob
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodGet, remoteURL, "/router/v1/node/site/download/jobs/"+jobID, nil, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return downloader.DownloadJob{}, unavailableError(handlers.capability)
	}
	response, found, err := handlers.downloader.Job(jobID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if !found {
		return downloader.DownloadJob{}, fmt.Errorf("download job was not found")
	}
	return response, nil
}

func (handlers *Handlers) downloadJobAction(ctx context.Context, nodeID string, jobID string, action string) (downloader.DownloadJob, error) {
	remoteURL, remote, err := handlers.downloadTarget(nodeID)
	if err != nil {
		return downloader.DownloadJob{}, err
	}
	if remote {
		var response downloader.DownloadJob
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/jobs/"+jobID+"/"+action, nil, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return downloader.DownloadJob{}, unavailableError(handlers.capability)
	}
	switch action {
	case "pause":
		return handlers.downloader.Pause(jobID)
	case "resume":
		return handlers.downloader.Resume(jobID)
	case "cancel":
		return handlers.downloader.Cancel(jobID)
	}
	return downloader.DownloadJob{}, fmt.Errorf("download job action is invalid")
}

func (handlers *Handlers) downloadLibrary(ctx context.Context, nodeID string) (siteapi.DownloadLibraryResponse, error) {
	remoteURL, remote, err := handlers.downloadTarget(nodeID)
	if err != nil {
		return siteapi.DownloadLibraryResponse{}, err
	}
	if remote {
		var response siteapi.DownloadLibraryResponse
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodGet, remoteURL, "/router/v1/node/site/download/library", nil, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return siteapi.DownloadLibraryResponse{}, unavailableError(handlers.capability)
	}
	artifacts, err := handlers.downloader.Artifacts()
	if err != nil {
		return siteapi.DownloadLibraryResponse{}, err
	}
	jobs, err := handlers.downloader.Jobs()
	if err != nil {
		return siteapi.DownloadLibraryResponse{}, err
	}
	return siteapi.DownloadLibraryResponse{Artifacts: artifacts, Jobs: jobs}, nil
}

func (handlers *Handlers) downloadRescan(ctx context.Context, nodeID string) (map[string]any, error) {
	remoteURL, remote, err := handlers.downloadTarget(nodeID)
	if err != nil {
		return nil, err
	}
	if remote {
		var response map[string]any
		err := handlers.deps.ClusterClient().JSON(ctx, http.MethodPost, remoteURL, "/router/v1/node/site/download/rescan", nil, &response)
		return response, err
	}
	if handlers.downloader == nil {
		return nil, unavailableError(handlers.capability)
	}
	artifacts, err := handlers.downloader.Rescan()
	if err != nil {
		return nil, err
	}
	return map[string]any{"artifacts": artifacts}, nil
}

func (handlers *Handlers) downloadTarget(nodeID string) (string, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || nodeID == handlers.deps.NodeID() {
		return "", false, nil
	}
	if handlers.deps.ClusterRole() != cluster.RoleMaster {
		return "", false, fmt.Errorf("selected download node is not local")
	}
	nodeURL := strings.TrimSpace(handlers.deps.NodeURLByID(nodeID))
	if nodeURL == "" {
		return "", false, fmt.Errorf("download node %q was not found", nodeID)
	}
	return nodeURL, true, nil
}

func unavailableError(capability downloader.Capability) error {
	if capability.Error != "" {
		return fmt.Errorf("downloader is unavailable: %s", capability.Error)
	}
	return fmt.Errorf("downloader is unavailable on this node")
}
