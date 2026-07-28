package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteModelAssetBinding(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetBinding(w, r)
	if !ok {
		return
	}
	target, err := service.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid config node")
		return
	}
	request.NodeID, request.NodeURL = target.nodeID, target.nodeURL
	if !target.local {
		var response map[string]string
		if err := service.clusterClient.JSON(r.Context(), http.MethodPost, target.nodeURL, "/router/v1/node/site/model-assets/bind", request, &response); err != nil {
			openai.WriteError(w, http.StatusBadGateway, "cluster_error", "asset binding failed")
			return
		}
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	service.bindLocalModelAsset(w, r, request)
}

func (service *Service) handleNodeModelAssetBinding(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetBinding(w, r)
	if !ok {
		return
	}
	service.bindLocalModelAsset(w, r, request)
}

func (service *Service) bindLocalModelAsset(w http.ResponseWriter, r *http.Request, request siteapi.ModelAssetBindingRequest) {
	if service.assetIndex == nil || service.downloader == nil || !modelassets.ValidHash(request.SHA256) {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "asset binding is unavailable")
		return
	}
	origin := modelassets.Origin{Repository: request.Repository, Commit: request.Commit, Path: request.RepositoryPath}
	if origin.URI() == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid Hugging Face origin")
		return
	}
	details, err := service.downloader.Repository(r.Context(), downloader.RepositoryRequest{Repository: origin.Repository, Revision: origin.Commit, Token: request.Token})
	if err != nil || details.Commit != origin.Commit {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Hugging Face origin could not be verified")
		return
	}
	verified := false
	for _, file := range details.Files {
		if file.Path == origin.Path && strings.EqualFold(file.LFSHash, request.SHA256) {
			verified = true
			break
		}
	}
	if !verified {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "candidate file does not have the expected SHA-256")
		return
	}
	if err := service.assetIndex.BindOrigin(request.SHA256, origin); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "model_asset_error", "asset origin could not be saved")
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]string{"sha256": request.SHA256, "hf": origin.URI()})
}

func decodeModelAssetBinding(w http.ResponseWriter, r *http.Request) (siteapi.ModelAssetBindingRequest, bool) {
	defer r.Body.Close()
	var request siteapi.ModelAssetBindingRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid binding request")
		return siteapi.ModelAssetBindingRequest{}, false
	}
	request.Token = strings.TrimSpace(request.Token)
	return request, true
}
