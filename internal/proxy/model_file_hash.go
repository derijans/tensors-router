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

func (assets *assetManager) handleSiteModelFileHash(w http.ResponseWriter, r *http.Request) {
	if !assets.deps.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, ok := decodeModelFileHashRequest(w, r)
	if !ok {
		return
	}
	response, err := assets.hashModelFile(r.Context(), request)
	if err != nil {
		assets.logger.Printf("model file hash failed node=%q error_type=%T", request.NodeID, err)
		openai.WriteError(w, http.StatusBadRequest, "model_file_hash_failed", "model file could not be hashed")
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (assets *assetManager) handleNodeModelFileHash(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelFileHashRequest(w, r)
	if !ok {
		return
	}
	response, err := assets.hashLocalModelFile(request)
	if err != nil {
		assets.logger.Printf("model file hash failed node=%q error_type=%T", request.NodeID, err)
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

func (assets *assetManager) hashModelFile(ctx context.Context, request siteapi.ModelFileHashRequest) (siteapi.ModelFileHashResponse, error) {
	target, err := assets.deps.configNodeTarget(request.NodeID, "")
	if err != nil {
		return siteapi.ModelFileHashResponse{}, err
	}
	request.NodeID = target.nodeID
	if target.local {
		return assets.hashLocalModelFile(request)
	}
	var response siteapi.ModelFileHashResponse
	err = assets.identity().client.JSON(ctx, http.MethodPost, target.nodeURL, "/router/v1/node/site/model-files/hash", request, &response)
	return response, err
}

func (assets *assetManager) hashLocalModelFile(request siteapi.ModelFileHashRequest) (siteapi.ModelFileHashResponse, error) {
	identity := assets.identity()
	if assets.index == nil {
		return siteapi.ModelFileHashResponse{}, fmt.Errorf("model asset index is unavailable")
	}
	if request.NodeID != "" && request.NodeID != identity.nodeID {
		return siteapi.ModelFileHashResponse{}, fmt.Errorf("model file belongs to another node")
	}
	models, err := assets.deps.localClusterModels()
	if err != nil {
		return siteapi.ModelFileHashResponse{}, err
	}
	files, err := inventory.Scan(assets.fileRoots, models, identity.nodeID)
	if err != nil {
		return siteapi.ModelFileHashResponse{}, err
	}
	requestedPath := filepath.Clean(request.Path)
	for _, file := range files {
		if file.Path != requestedPath {
			continue
		}
		asset, indexErr := assets.index.IndexFile(file.Path)
		if indexErr != nil {
			return siteapi.ModelFileHashResponse{}, indexErr
		}
		return siteapi.ModelFileHashResponse{NodeID: identity.nodeID, Path: file.Path, SHA256: asset.SHA256}, nil
	}
	return siteapi.ModelFileHashResponse{}, fmt.Errorf("model file is not in configured inventory")
}
