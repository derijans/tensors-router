package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

type configNodeTarget struct {
	nodeID  string
	nodeURL string
	local   bool
}

func (service *Service) handleSiteConfigFilePreview(w http.ResponseWriter, r *http.Request) {
	service.handleSiteConfigFileSave(w, r, true)
}

func (service *Service) handleSiteConfigFileApply(w http.ResponseWriter, r *http.Request) {
	service.handleSiteConfigFileSave(w, r, false)
}

func (service *Service) handleSiteConfigFileSave(w http.ResponseWriter, r *http.Request, dryRun bool) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.ConfigFileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := service.saveConfigFile(r.Context(), request, dryRun)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, result)
}

func (service *Service) handleSiteConfigFileDelete(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var request siteapi.ConfigFileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := service.deleteConfigFile(r.Context(), request)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, result)
}

func (service *Service) handleNodeConfigFilePreview(w http.ResponseWriter, r *http.Request) {
	service.handleNodeConfigFileSave(w, r, true)
}

func (service *Service) handleNodeConfigFileApply(w http.ResponseWriter, r *http.Request) {
	service.handleNodeConfigFileSave(w, r, false)
}

func (service *Service) handleNodeConfigFileSave(w http.ResponseWriter, r *http.Request, dryRun bool) {
	var request siteapi.ConfigFileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := service.saveLocalConfigFile(request, dryRun)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if !dryRun {
		if err := service.refreshLocalRegistry(); err != nil {
			openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
			return
		}
	}
	openai.WriteJSON(w, http.StatusOK, result)
}

func (service *Service) handleNodeConfigFileDelete(w http.ResponseWriter, r *http.Request) {
	var request siteapi.ConfigFileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	result, err := service.deleteLocalConfigFile(request)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.refreshLocalRegistry(); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "site_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, result)
}

func (service *Service) saveConfigFile(ctx context.Context, request siteapi.ConfigFileRequest, dryRun bool) (siteapi.ConfigFileResponse, error) {
	target, err := service.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	request.NodeID = target.nodeID
	request.NodeURL = target.nodeURL
	if target.local {
		result, err := service.saveLocalConfigFile(request, dryRun)
		if err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
		if !dryRun {
			if err := service.refreshLocalRegistry(); err != nil {
				return siteapi.ConfigFileResponse{}, err
			}
		}
		return result, nil
	}
	path := "/router/v1/node/site/config-file/preview"
	if !dryRun {
		path = "/router/v1/node/site/config-file/apply"
	}
	var result siteapi.ConfigFileResponse
	if err := service.clusterClient.JSON(ctx, http.MethodPost, target.nodeURL, path, request, &result); err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	if !dryRun {
		if err := service.refreshRemoteConfigNode(ctx, target.nodeURL); err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
	}
	return result, nil
}

func (service *Service) deleteConfigFile(ctx context.Context, request siteapi.ConfigFileRequest) (siteapi.ConfigFileResponse, error) {
	target, err := service.configNodeTarget(request.NodeID, request.NodeURL)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	request.NodeID = target.nodeID
	request.NodeURL = target.nodeURL
	if target.local {
		result, err := service.deleteLocalConfigFile(request)
		if err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
		if err := service.refreshLocalRegistry(); err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
		return result, nil
	}
	var result siteapi.ConfigFileResponse
	if err := service.clusterClient.JSON(ctx, http.MethodDelete, target.nodeURL, "/router/v1/node/site/config-file", request, &result); err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	if err := service.refreshRemoteConfigNode(ctx, target.nodeURL); err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	return result, nil
}

func (service *Service) saveLocalConfigFile(request siteapi.ConfigFileRequest, dryRun bool) (siteapi.ConfigFileResponse, error) {
	options, err := cook.NormalizedOptions(request.Options)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	if options == nil {
		options = cook.Options{}
	}
	id, filename, err := configFileIdentity(request)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	target, err := service.localConfigFileTarget(filename)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	exists := false
	if _, err := os.Stat(target); err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		return siteapi.ConfigFileResponse{}, err
	}
	if exists && !request.Overwrite {
		return siteapi.ConfigFileResponse{}, fmt.Errorf("config %q already exists", filename)
	}
	if !dryRun {
		if err := os.MkdirAll(service.configDir, 0o755); err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
		content, err := json.MarshalIndent(options, "", "  ")
		if err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return siteapi.ConfigFileResponse{}, err
		}
	}
	return siteapi.ConfigFileResponse{
		NodeID:         service.nodeID,
		NodeURL:        service.nodeURL,
		ID:             id,
		Filename:       filename,
		WouldOverwrite: exists,
		Options:        options,
	}, nil
}

func (service *Service) deleteLocalConfigFile(request siteapi.ConfigFileRequest) (siteapi.ConfigFileResponse, error) {
	id, filename, err := configFileIdentity(request)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	target, err := service.localConfigFileTarget(filename)
	if err != nil {
		return siteapi.ConfigFileResponse{}, err
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return siteapi.ConfigFileResponse{}, fmt.Errorf("config %q was not found", filename)
		}
		return siteapi.ConfigFileResponse{}, err
	}
	return siteapi.ConfigFileResponse{
		NodeID:   service.nodeID,
		NodeURL:  service.nodeURL,
		ID:       id,
		Filename: filename,
		Deleted:  true,
	}, nil
}

func (service *Service) configNodeTarget(nodeID string, nodeURL string) (configNodeTarget, error) {
	nodeID = strings.TrimSpace(nodeID)
	nodeURL = strings.TrimSpace(nodeURL)
	if nodeID == "" {
		nodeID = service.nodeID
	}
	if nodeID == service.nodeID {
		if nodeURL != "" && service.nodeURL != "" && !cluster.BaseURLEqual(nodeURL, service.nodeURL) {
			return configNodeTarget{}, fmt.Errorf("node url for %q does not match the registered node", nodeID)
		}
		return configNodeTarget{nodeID: service.nodeID, nodeURL: service.nodeURL, local: true}, nil
	}
	resolvedNodeURL := service.nodeURLByID()[nodeID]
	if resolvedNodeURL == "" {
		return configNodeTarget{}, fmt.Errorf("node url for %q is required", nodeID)
	}
	if nodeURL != "" && !cluster.BaseURLEqual(nodeURL, resolvedNodeURL) {
		return configNodeTarget{}, fmt.Errorf("node url for %q does not match the registered node", nodeID)
	}
	return configNodeTarget{nodeID: nodeID, nodeURL: resolvedNodeURL}, nil
}

func (service *Service) refreshRemoteConfigNode(ctx context.Context, nodeURL string) error {
	if service.registry == nil {
		return nil
	}
	snapshot, err := service.clusterClient.FetchSnapshot(ctx, nodeURL)
	if err != nil {
		service.registry.MarkNodeURLHealth(nodeURL, false)
		return nil
	}
	if snapshot.NodeURL == "" {
		snapshot.NodeURL = nodeURL
	}
	return service.registry.UpdateNode(snapshot)
}

func configFileIdentity(request siteapi.ConfigFileRequest) (string, string, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		id = strings.TrimSuffix(strings.TrimSpace(request.Filename), filepath.Ext(request.Filename))
	}
	id, err := cook.SanitizedID(id)
	if err != nil {
		return "", "", err
	}
	filename := id + ".kcpps"
	return id, filename, nil
}

func (service *Service) localConfigFileTarget(filename string) (string, error) {
	if strings.TrimSpace(service.configDir) == "" {
		return "", fmt.Errorf("config dir is required")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" || filename != filepath.Base(filename) || filepath.Ext(filename) != ".kcpps" || !filepath.IsLocal(filename) {
		return "", fmt.Errorf("config filename is invalid")
	}
	configDir, err := filepath.Abs(service.configDir)
	if err != nil {
		return "", err
	}
	configDir = filepath.Clean(configDir)
	target := filepath.Clean(filepath.Join(configDir, filename))
	if !configPathInsideRoot(configDir, target) {
		return "", fmt.Errorf("config filename is invalid")
	}
	return target, nil
}

func configPathInsideRoot(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." && !filepath.IsAbs(relative))
}
