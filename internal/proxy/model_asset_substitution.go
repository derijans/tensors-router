package proxy

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"tensors-router/internal/atomicfile"
	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (service *Service) handleSiteModelAssetSubstitution(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetSubstitution(w, r)
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
		if err := service.clusterClient.JSON(r.Context(), http.MethodPost, target.nodeURL, "/router/v1/node/site/model-assets/substitute", request, &response); err != nil {
			openai.WriteError(w, http.StatusBadGateway, "cluster_error", "model asset substitution failed")
			return
		}
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	service.substituteLocalModelAsset(w, r, request)
}

func (service *Service) handleNodeModelAssetSubstitution(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetSubstitution(w, r)
	if !ok {
		return
	}
	service.substituteLocalModelAsset(w, r, request)
}

func (service *Service) substituteLocalModelAsset(w http.ResponseWriter, r *http.Request, request siteapi.ModelAssetSubstitutionRequest) {
	if !request.Confirm || service.assetIndex == nil || service.downloader == nil || !modelassets.ValidHash(request.SHA256) || !modelassets.ValidHash(request.ExpectedSHA256) || request.SHA256 == request.ExpectedSHA256 {
		openai.WriteError(w, http.StatusBadRequest, "confirmation_required", "explicit model replacement confirmation is required")
		return
	}
	origin := modelassets.Origin{Repository: request.Repository, Commit: request.Commit, Path: request.RepositoryPath}
	if origin.URI() == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid Hugging Face origin")
		return
	}
	details, err := service.downloader.Repository(r.Context(), downloader.RepositoryRequest{Repository: origin.Repository, Revision: origin.Commit, Token: request.Token})
	if err != nil || details.Commit != origin.Commit || !repositoryFileHasHash(details, origin.Path, request.SHA256) {
		openai.WriteError(w, http.StatusBadRequest, "model_asset_mismatch", "replacement candidate could not be verified")
		return
	}
	_, _, target, err := service.modelAssetConfigTarget(siteapi.ModelAssetConfigRequest{ID: request.ID, Filename: request.Filename})
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid config target")
		return
	}
	unlock := service.lockModelAssetConfig(target)
	defer unlock()
	content, err := os.ReadFile(target)
	if err != nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "config was not found")
		return
	}
	fingerprint := sha256.Sum256(content)
	replacement, err := modelassets.Substitute(content, request.Field, request.Position, request.ExpectedSHA256, request.SHA256, pathBase(origin.Path), origin)
	if err != nil {
		openai.WriteError(w, http.StatusConflict, "model_asset_conflict", "config no longer contains the expected model asset")
		return
	}
	current, err := os.ReadFile(target)
	if err != nil || sha256.Sum256(current) != fingerprint {
		openai.WriteError(w, http.StatusConflict, "model_asset_conflict", "config changed during model replacement")
		return
	}
	if err := service.assetIndex.BindOrigin(request.SHA256, origin); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "model_asset_error", "replacement origin could not be saved")
		return
	}
	if err := atomicfile.Write(target, replacement, 0o644); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "model_asset_error", "config replacement could not be committed")
		return
	}
	_ = service.refreshLocalRegistry()
	openai.WriteJSON(w, http.StatusOK, map[string]string{"sha256": request.SHA256, "hf": origin.URI()})
}

func decodeModelAssetSubstitution(w http.ResponseWriter, r *http.Request) (siteapi.ModelAssetSubstitutionRequest, bool) {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var request siteapi.ModelAssetSubstitutionRequest
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid model asset substitution")
		return siteapi.ModelAssetSubstitutionRequest{}, false
	}
	request.Token = strings.TrimSpace(request.Token)
	return request, true
}

func repositoryFileHasHash(details downloader.RepositoryDetails, repositoryPath string, hash string) bool {
	for _, file := range details.Files {
		if file.Path == repositoryPath && strings.EqualFold(file.LFSHash, hash) {
			return true
		}
	}
	return false
}

func pathBase(value string) string {
	parts := strings.Split(value, "/")
	return parts[len(parts)-1]
}
