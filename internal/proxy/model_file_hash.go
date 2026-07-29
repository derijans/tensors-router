package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"tensors-router/internal/inventory"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteModelFileHash(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, ok := decodeModelFileHashRequest(w, r)
	if !ok {
		return
	}
	response, err := service.hashModelFile(r.Context(), request)
	if err != nil {
		service.logger.Printf("model file hash failed node=%q error_type=%T", request.NodeID, err)
		openai.WriteError(w, http.StatusBadRequest, "model_file_hash_failed", "model file could not be hashed")
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeModelFileHash(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelFileHashRequest(w, r)
	if !ok {
		return
	}
	response, err := service.hashLocalModelFile(request)
	if err != nil {
		service.logger.Printf("model file hash failed node=%q error_type=%T", request.NodeID, err)
		openai.WriteError(w, http.StatusBadRequest, "model_file_hash_failed", "model file could not be hashed")
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func decodeModelFileHashRequest(w http.ResponseWriter, r *http.Request) (siteapi.ModelFileHashRequest, bool) {
	if r.Body == nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body is required")
		return siteapi.ModelFileHashRequest{}, false
	}
	defer r.Body.Close()
	var request siteapi.ModelFileHashRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Path) == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid model file hash request")
		return siteapi.ModelFileHashRequest{}, false
	}
	return request, true
}

func (service *Service) hashModelFile(ctx context.Context, request siteapi.ModelFileHashRequest) (siteapi.ModelFileHashResponse, error) {
	target, err := service.configNodeTarget(request.NodeID, "")
	if err != nil {
		return siteapi.ModelFileHashResponse{}, err
	}
	request.NodeID = target.nodeID
	if target.local {
		return service.hashLocalModelFile(request)
	}
	var response siteapi.ModelFileHashResponse
	err = service.clusterClient.JSON(ctx, http.MethodPost, target.nodeURL, "/router/v1/node/site/model-files/hash", request, &response)
	return response, err
}

func (service *Service) hashLocalModelFile(request siteapi.ModelFileHashRequest) (siteapi.ModelFileHashResponse, error) {
	if service.assetIndex == nil {
		return siteapi.ModelFileHashResponse{}, fmt.Errorf("model asset index is unavailable")
	}
	if request.NodeID != "" && request.NodeID != service.nodeID {
		return siteapi.ModelFileHashResponse{}, fmt.Errorf("model file belongs to another node")
	}
	models, err := service.localClusterModels()
	if err != nil {
		return siteapi.ModelFileHashResponse{}, err
	}
	files, err := inventory.Scan(service.fileRoots, models, service.nodeID)
	if err != nil {
		return siteapi.ModelFileHashResponse{}, err
	}
	requestedPath := filepath.Clean(request.Path)
	for _, file := range files {
		if file.Path != requestedPath {
			continue
		}
		asset, indexErr := service.assetIndex.IndexFile(file.Path)
		if indexErr != nil {
			return siteapi.ModelFileHashResponse{}, indexErr
		}
		return siteapi.ModelFileHashResponse{NodeID: service.nodeID, Path: file.Path, SHA256: asset.SHA256}, nil
	}
	return siteapi.ModelFileHashResponse{}, fmt.Errorf("model file is not in configured inventory")
}
