package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"tensors-router/internal/atomicfile"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

func (assets *assetManager) handleSiteModelAssetExport(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetConfigRequest(w, r)
	if !ok {
		return
	}
	response, err := assets.exportModelAssetConfig(r.Context(), request)
	if err != nil {
		assets.logger.Printf("portable model export failed config=%q error_type=%T", request.ID, err)
		openai.WriteError(w, http.StatusBadRequest, "model_asset_export_failed", "portable model export failed")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", response.Filename))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response.Content)
}

func (assets *assetManager) handleNodeModelAssetExport(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetConfigRequest(w, r)
	if !ok {
		return
	}
	response, err := assets.exportLocalModelAssetConfig(request)
	if err != nil {
		assets.logger.Printf("portable model export failed config=%q error_type=%T", request.ID, err)
		openai.WriteError(w, http.StatusBadRequest, "model_asset_export_failed", "portable model export failed")
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (assets *assetManager) handleSiteModelAssetResolve(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetConfigRequest(w, r)
	if !ok {
		return
	}
	response, err := assets.resolveModelAssetConfig(r.Context(), request)
	if err != nil {
		assets.logger.Printf("model asset resolution failed config=%q error_type=%T", request.ID, err)
		openai.WriteError(w, http.StatusServiceUnavailable, "model_asset_unavailable", "model asset is unavailable")
		return
	}
	status := http.StatusOK
	for _, result := range response.Results {
		if !result.Resolved {
			status = http.StatusServiceUnavailable
			break
		}
	}
	openai.WriteJSON(w, status, response)
}

func (assets *assetManager) handleNodeModelAssetResolve(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeModelAssetConfigRequest(w, r)
	if !ok {
		return
	}
	response, err := assets.resolveLocalModelAssetConfig(request)
	if err != nil {
		assets.logger.Printf("model asset resolution failed config=%q error_type=%T", request.ID, err)
		openai.WriteError(w, http.StatusServiceUnavailable, "model_asset_unavailable", "model asset is unavailable")
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (assets *assetManager) handleSiteModelAssetResolveBatch(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var requests []siteapi.ModelAssetConfigRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&requests); err != nil || len(requests) == 0 || len(requests) > 128 {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid model asset batch")
		return
	}
	responses := make([]siteapi.ModelAssetConfigResponse, 0, len(requests))
	for _, request := range requests {
		response, err := assets.resolveModelAssetConfig(r.Context(), request)
		if err != nil {
			responses = append(responses, siteapi.ModelAssetConfigResponse{ID: request.ID, Results: []siteapi.ModelAssetFieldResult{{Failure: "resolution failed"}}})
			continue
		}
		responses = append(responses, response)
	}
	openai.WriteJSON(w, http.StatusOK, responses)
}

func (assets *assetManager) handleSiteModelAssetLookup(w http.ResponseWriter, r *http.Request) {
	identity := assets.identity()
	if assets.index == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "model asset index is unavailable")
		return
	}
	hash := strings.TrimPrefix(r.URL.Path, "/router/v1/site/model-assets/")
	if !modelassets.ValidHash(hash) || strings.Contains(hash, "/") {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset hash")
		return
	}
	response := map[string]any{"sha256": hash, "available": false, "nodes": []string{}}
	if asset, found := assets.index.Lookup(hash); found {
		response["available"] = true
		response["filename"] = asset.Filename
		response["size"] = asset.Size
		response["nodes"] = []string{identity.nodeID}
	}
	if origin, found := assets.index.Origin(hash); found {
		response["origin"] = origin.URI()
	}
	for _, source := range assets.coordinatedAssetSources(hash) {
		if source.Filename != "" && source.NodeURL != identity.nodeURL {
			response["available"] = true
			existing := response["nodes"].([]string)
			response["nodes"] = append(existing, "peer")
		}
		if _, found := response["origin"]; !found && source.Origin != "" {
			response["origin"] = source.Origin
		}
	}
	if response["available"].(bool) || response["origin"] != nil {
		openai.WriteJSON(w, http.StatusOK, response)
		return
	}
	openai.WriteError(w, http.StatusNotFound, "not_found", "model asset was not found")
}

func decodeModelAssetConfigRequest(w http.ResponseWriter, r *http.Request) (siteapi.ModelAssetConfigRequest, bool) {
	if r.Body == nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body is required")
		return siteapi.ModelAssetConfigRequest{}, false
	}
	defer r.Body.Close()
	var request siteapi.ModelAssetConfigRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return siteapi.ModelAssetConfigRequest{}, false
	}
	return request, true
}

func (assets *assetManager) exportModelAssetConfig(ctx context.Context, request siteapi.ModelAssetConfigRequest) (siteapi.ModelAssetConfigResponse, error) {
	target, err := assets.deps.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	request.NodeID, request.NodeURL = target.nodeID, target.nodeURL
	if target.local {
		return assets.exportLocalModelAssetConfig(request)
	}
	var response siteapi.ModelAssetConfigResponse
	err = assets.identity().client.JSON(ctx, http.MethodPost, target.nodeURL, "/router/v1/node/site/model-assets/export", request, &response)
	return response, err
}

func (assets *assetManager) exportLocalModelAssetConfig(request siteapi.ModelAssetConfigRequest) (siteapi.ModelAssetConfigResponse, error) {
	if assets.index == nil {
		return siteapi.ModelAssetConfigResponse{}, fmt.Errorf("model asset index is unavailable")
	}
	id, filename, target, err := assets.modelAssetConfigTarget(request)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	exported, err := modelassets.Export(content, func(path string) (string, error) {
		asset, indexErr := assets.index.IndexFile(path)
		return asset.SHA256, indexErr
	}, assets.index.Origin)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	return siteapi.ModelAssetConfigResponse{ID: id, Filename: filename, Content: exported}, nil
}

type activeConfigResolution struct {
	ready chan struct{}
	done  chan struct{}
	job   modelassets.ResolutionJob
	err   error
}

func (assets *assetManager) ensure(ctx context.Context, filename string) error {
	if assets.index == nil {
		return nil
	}
	request := siteapi.ModelAssetConfigRequest{Filename: filename}
	_, _, target, err := assets.modelAssetConfigTarget(request)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	unresolved, err := modelassets.UnresolvedFields(content)
	if err != nil || unresolved == 0 {
		return err
	}
	job, active, err := assets.startSharedLocalModelAssetJob(request, target)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-active.done:
	}
	persistedJob, found, err := assets.index.ResolutionJob(job.ID)
	if err != nil || !found || persistedJob.State != modelassets.JobCompleted {
		return fmt.Errorf("model asset unavailable")
	}
	return nil
}

func (assets *assetManager) resolveModelAssetConfig(ctx context.Context, request siteapi.ModelAssetConfigRequest) (siteapi.ModelAssetConfigResponse, error) {
	target, err := assets.deps.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	request.NodeID, request.NodeURL = target.nodeID, target.nodeURL
	if target.local {
		return assets.resolveLocalModelAssetConfig(request)
	}
	var response siteapi.ModelAssetConfigResponse
	err = assets.identity().client.JSON(ctx, http.MethodPost, target.nodeURL, "/router/v1/node/site/model-assets/resolve", request, &response)
	return response, err
}

func (assets *assetManager) resolveLocalModelAssetConfig(request siteapi.ModelAssetConfigRequest) (siteapi.ModelAssetConfigResponse, error) {
	if assets.index == nil {
		return siteapi.ModelAssetConfigResponse{}, fmt.Errorf("model asset index is unavailable")
	}
	id, filename, target, err := assets.modelAssetConfigTarget(request)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	unlock := assets.lockModelAssetConfig(target)
	defer unlock()
	content, err := os.ReadFile(target)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	fingerprint := sha256.Sum256(content)
	resolved, err := modelassets.ResolveDetailed(content, assets.resolveAssetReferenceDetailed)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	response := siteapi.ModelAssetConfigResponse{ID: id, Filename: filename, Results: assetFieldResults(resolved.Fields)}
	if string(resolved.Content) == string(content) {
		return response, nil
	}
	current, err := os.ReadFile(target)
	if err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	if sha256.Sum256(current) != fingerprint {
		return siteapi.ModelAssetConfigResponse{}, fmt.Errorf("config changed while model assets were resolving")
	}
	if err := atomicfile.Write(target, resolved.Content, 0o644); err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	if err := assets.deps.refreshLocalRegistry(); err != nil {
		return siteapi.ModelAssetConfigResponse{}, err
	}
	return response, nil
}

func (assets *assetManager) lockModelAssetConfig(target string) func() {
	candidate := &sync.Mutex{}
	value, _ := assets.configLocks.LoadOrStore(target, candidate)
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func (assets *assetManager) modelAssetConfigTarget(request siteapi.ModelAssetConfigRequest) (string, string, string, error) {
	id, filename, err := configFileIdentity(siteapi.ConfigFileRequest{ID: request.ID, Filename: request.Filename})
	if err != nil {
		return "", "", "", err
	}
	target, err := assets.deps.localConfigFileTarget(filename)
	return id, filename, target, err
}

func assetFieldResults(values []modelassets.FieldResult) []siteapi.ModelAssetFieldResult {
	results := make([]siteapi.ModelAssetFieldResult, len(values))
	for index, value := range values {
		results[index] = siteapi.ModelAssetFieldResult{Field: value.Field, Hash: value.Hash, Resolved: value.Resolved, Failure: value.Failure, Source: value.Source, Verification: value.Verification, Commit: value.Commit}
	}
	return results
}
