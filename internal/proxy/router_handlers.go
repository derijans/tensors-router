package proxy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/unloadpolicy"
)

type modelControlRequest struct {
	Model  string `json:"model"`
	Target string `json:"target"`
}

func (service *Service) handleRouterEndpoint(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/router/v1/vllm/"):
		service.handleVLLMAdmin(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/inventory":
		service.handleSiteInventory(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/nodes/state":
		service.handleSiteNodeState(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/nodes/unload":
		service.handleSiteNodeUnload(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/nodes/backends/init":
		service.handleSiteBackendInitialization(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/nodes/backends/init/cancel":
		service.handleSiteBackendInitializationCancel(w, r)
	case (r.Method == http.MethodGet || r.Method == http.MethodPost) && r.URL.Path == "/router/v1/site/nodes/backends/launch-options":
		service.handleSiteBackendLaunchOptions(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/models/state":
		service.handleSiteModelState(w, r)
	case r.URL.Path == "/router/v1/site/routing-groups":
		service.handleSiteRoutingGroups(w, r)
	case r.URL.Path == "/router/v1/site/separate-runtimes":
		service.handleSiteSeparateRuntimes(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/download/capabilities":
		service.handleSiteDownloadCapabilities(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/download/search":
		service.handleSiteDownloadSearch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/download/search-page":
		service.handleSiteDownloadSearchPage(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/download/repository":
		service.handleSiteDownloadRepository(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/download/plan":
		service.handleSiteDownloadPlan(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/download/jobs":
		service.handleSiteDownloadCreateJob(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/events"):
		service.handleSiteDownloadEvents(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/site/download/jobs/"):
		service.handleSiteDownloadJob(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/router/v1/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/pause"):
		service.handleSiteDownloadPause(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/router/v1/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/resume"):
		service.handleSiteDownloadResume(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/router/v1/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		service.handleSiteDownloadCancel(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/download/library":
		service.handleSiteDownloadLibrary(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/download/rescan":
		service.handleSiteDownloadRescan(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/webuis":
		service.handleSiteWebUIs(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/webuis/session":
		service.handleSiteWebUISession(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/webuis/load":
		service.handleSiteWebUILoad(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/benchmarks":
		service.handleBenchmarks(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/benchmarks/run":
		service.handleBenchmarkRun(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/analytics":
		service.handleSiteAnalytics(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/load-captures":
		service.handleSiteLoadCaptures(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/site/load-captures/"):
		service.handleSiteLoadCaptureRecord(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/site/load-errors":
		service.handleSiteLoadErrors(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/cook/preview":
		service.handleSiteCookPreview(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/cook/apply":
		service.handleSiteCookApply(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/router/v1/site/cook/"):
		service.handleSiteCookDelete(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/config-file/preview":
		service.handleSiteConfigFilePreview(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/config-file/apply":
		service.handleSiteConfigFileApply(w, r)
	case r.Method == http.MethodDelete && r.URL.Path == "/router/v1/site/config-file":
		service.handleSiteConfigFileDelete(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/export":
		service.handleSiteModelAssetExport(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-files/hash":
		service.handleSiteModelFileHash(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/resolve":
		service.handleSiteModelAssetResolve(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/resolve-batch":
		service.handleSiteModelAssetResolveBatch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/jobs":
		service.handleSiteModelAssetCreateJob(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/bind":
		service.handleSiteModelAssetBinding(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/candidates":
		service.handleSiteModelAssetCandidates(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/site/model-assets/substitute":
		service.handleSiteModelAssetSubstitution(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/site/model-assets/jobs/"):
		service.handleSiteModelAssetJob(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/site/model-assets/"):
		service.handleSiteModelAssetLookup(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/models":
		service.handleRouterModels(w)
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/models":
		if service.requireClusterToken(w, r) {
			service.handleNodeModels(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/mcp":
		if service.requireClusterToken(w, r) {
			service.handleNodeMCP(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/models/state":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelState(w, r)
		}
	case (r.Method == http.MethodGet || r.Method == http.MethodPost) && r.URL.Path == nodeSeparateRuntimesPath:
		if service.requireClusterToken(w, r) {
			service.handleNodeSeparateRuntimes(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/state":
		if service.requireClusterToken(w, r) {
			service.handleNodeState(w)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/state/unload":
		if service.requireClusterToken(w, r) {
			service.handleNodeStateUnload(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/backends/init":
		if service.requireClusterToken(w, r) {
			service.handleNodeBackendInitialization(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/backends/init/cancel":
		if service.requireClusterToken(w, r) {
			service.handleNodeBackendInitializationCancel(w, r)
		}
	case (r.Method == http.MethodGet || r.Method == http.MethodPost) && r.URL.Path == "/router/v1/node/backends/launch-options":
		if service.requireClusterToken(w, r) {
			service.handleNodeBackendLaunchOptions(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-assets/resolve":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetResolve(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-assets/export":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetExport(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-files/hash":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelFileHash(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-assets/jobs":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetCreateJob(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-assets/bind":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetBinding(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-assets/candidates":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetCandidates(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/model-assets/substitute":
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetSubstitution(w, r)
		}
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/node/site/model-assets/jobs/"):
		if service.requireClusterToken(w, r) {
			service.handleNodeModelAssetJob(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/assets/lookup":
		if service.requireClusterToken(w, r) {
			service.handleNodeAssetLookup(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/assets/lookup-cluster":
		if service.requireClusterToken(w, r) {
			service.handleNodeClusterAssetLookup(w, r)
		}
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/node/assets/"):
		if service.requireClusterToken(w, r) {
			service.handleNodeAssetStream(w, r)
		}
	case strings.HasPrefix(r.URL.Path, "/router/v1/node/inference/"):
		if service.requireClusterToken(w, r) {
			service.handleNodeInference(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/inventory":
		if service.requireClusterToken(w, r) {
			service.handleNodeSiteInventory(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/download/capabilities":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadCapabilities(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/download/search":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadSearch(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/download/search-page":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadSearchPage(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/download/repository":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadRepository(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/download/plan":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadPlan(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/download/jobs":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadCreateJob(w, r)
		}
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/node/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/events"):
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadEvents(w, r)
		}
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/node/site/download/jobs/"):
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadJob(w, r)
		}
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/router/v1/node/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/pause"):
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadPause(w, r)
		}
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/router/v1/node/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/resume"):
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadResume(w, r)
		}
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/router/v1/node/site/download/jobs/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadCancel(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/download/library":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadLibrary(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/download/rescan":
		if service.requireClusterToken(w, r) {
			service.handleNodeDownloadRescan(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/site/webuis":
		if service.requireClusterToken(w, r) {
			service.handleNodeSiteWebUIs(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/runtime-status":
		if service.requireClusterToken(w, r) {
			openai.WriteJSON(w, http.StatusOK, service.localRuntimeStatus())
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/offload/grant":
		if service.requireClusterToken(w, r) {
			service.handleNodeOffloadGrant(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/offload/request":
		if service.requireClusterToken(w, r) {
			service.handleNodeOffloadRequest(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/webuis/load":
		if service.requireClusterToken(w, r) {
			service.handleNodeSiteWebUILoad(w, r)
		}
	case strings.HasPrefix(r.URL.Path, nodeWebUIProxyPrefix):
		if service.requireClusterToken(w, r) {
			service.handleNodeWebUIProxy(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/benchmarks":
		if service.requireClusterToken(w, r) {
			service.handleNodeBenchmarks(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/benchmarks/run":
		if service.requireClusterToken(w, r) {
			service.handleNodeBenchmarkRun(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/analytics":
		if service.requireClusterToken(w, r) {
			service.handleNodeAnalytics(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/load-captures":
		if service.requireClusterToken(w, r) {
			service.handleNodeLoadCaptures(w, r)
		}
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/router/v1/node/load-captures/"):
		if service.requireClusterToken(w, r) {
			service.handleNodeLoadCaptureRecord(w, r)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/router/v1/node/load-errors":
		if service.requireClusterToken(w, r) {
			service.handleNodeLoadErrors(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/configs":
		if service.requireClusterToken(w, r) {
			service.handleNodeSiteConfigs(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/config-file/preview":
		if service.requireClusterToken(w, r) {
			service.handleNodeConfigFilePreview(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/site/config-file/apply":
		if service.requireClusterToken(w, r) {
			service.handleNodeConfigFileApply(w, r)
		}
	case r.Method == http.MethodDelete && r.URL.Path == "/router/v1/node/site/config-file":
		if service.requireClusterToken(w, r) {
			service.handleNodeConfigFileDelete(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/register":
		if service.requireClusterToken(w, r) {
			service.handleNodeRegister(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/load":
		if service.requireClusterToken(w, r) {
			service.handleRouterLoad(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/node/unload":
		if service.requireClusterToken(w, r) {
			service.handleRouterUnload(w, r)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/load":
		service.handleRouterLoad(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/unload":
		service.handleRouterUnload(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/router/v1/shutdown":
		service.handleRouterShutdown(w)
	default:
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (service *Service) handleNodeInference(w http.ResponseWriter, r *http.Request) {
	path, ok := nodeInferencePath(r.URL.Path)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	forwarded := r.Clone(r.Context())
	forwardedURL := *r.URL
	forwardedURL.Path = path
	forwarded.URL = &forwardedURL
	forwarded.Header = r.Header.Clone()
	forwarded.Header.Del("Authorization")
	borrowed := strings.TrimSpace(forwarded.Header.Get(offloadMarkerHeader)) != ""
	forwarded.Header.Del(offloadMarkerHeader)
	if borrowed {
		forwarded = markBorrowedRequest(forwarded)
	}
	service.ServeHTTP(w, forwarded)
}

func nodeInferencePath(requestPath string) (string, bool) {
	path := strings.TrimPrefix(requestPath, "/router/v1/node/inference")
	if !isLocalInferencePath(path) {
		return "", false
	}
	return path, isTextPath(path) || isImagePath(path) || isVoicePath(path) || isMusicPath(path)
}

func isLocalInferencePath(path string) bool {
	return len(path) > 0 && path[0] == '/' && (len(path) == 1 || (path[1] != '/' && path[1] != '\\'))
}

func (service *Service) handleRouterModels(w http.ResponseWriter) {
	if service.registry != nil {
		openai.WriteJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   service.withBenchmarks(service.registry.Models()),
		})
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   service.withBenchmarks(cluster.LocalModelsWithBackendMode(models, "local", "", cluster.SourceLocal, service.backendMode)),
	})
}

func (service *Service) handleNodeModels(w http.ResponseWriter, r *http.Request) {
	if service.registry != nil {
		snapshot := service.registry.Snapshot()
		snapshot.Models = service.withBenchmarks(service.modelsWithRuntimeState(r.Context(), snapshot.Models))
		openai.WriteJSON(w, http.StatusOK, snapshot)
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, cluster.Snapshot{
		NodeID: "local",
		Models: service.withBenchmarks(cluster.LocalModelsWithBackendMode(models, "local", "", cluster.SourceLocal, service.backendMode)),
	})
}

func (service *Service) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	if service.registry == nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", "cluster registry is not enabled")
		return
	}
	var snapshot cluster.Snapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.validateRegisteredNodeURL(snapshot.NodeURL); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	if err := service.registry.UpdateNode(snapshot); err != nil {
		if cluster.ErrorCode(err) == cluster.ErrorCodeDuplicateNode {
			openai.WriteErrorCode(w, http.StatusConflict, "cluster_error", cluster.ErrorCodeDuplicateNode, err.Error())
			return
		}
		openai.WriteError(w, http.StatusBadRequest, "cluster_error", err.Error())
		return
	}
	if err := service.clusterClient.AllowBaseURLs(snapshot.NodeURL); err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "cluster_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) validateRegisteredNodeURL(nodeURL string) error {
	nodeURL = strings.TrimSpace(nodeURL)
	if nodeURL == "" {
		return fmt.Errorf("node url is required")
	}
	if !configuredBaseURL(nodeURL, service.slaveURLs) {
		return fmt.Errorf("node url %q is not configured", nodeURL)
	}
	return nil
}

func configuredBaseURL(nodeURL string, configured []string) bool {
	for _, value := range configured {
		if cluster.BaseURLEqual(nodeURL, value) {
			return true
		}
	}
	return false
}

func (service *Service) handleRouterLoad(w http.ResponseWriter, r *http.Request) {
	if service.rejectModelLoadWhileDraining(w) {
		return
	}
	control, err := readModelControlRequest(r, true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()

	if err := service.loadPublicModel(ctx, control.Model); err != nil {
		writeRouterLoadError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeRouterLoadError(w http.ResponseWriter, err error) {
	diagnostic, ok := backenddiagnostic.FromError(err)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusBadGateway, struct {
		Error             openai.ErrorDetail           `json:"error"`
		BackendDiagnostic backenddiagnostic.Diagnostic `json:"backend_diagnostic"`
	}{
		Error:             openai.ErrorDetail{Message: err.Error(), Type: "backend_error"},
		BackendDiagnostic: diagnostic,
	})
}

func (service *Service) handleRouterUnload(w http.ResponseWriter, r *http.Request) {
	control, err := readModelControlRequest(r, false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancel()

	target, err := unloadpolicy.ResolveTarget(control.Target)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := service.unloadPublicModel(ctx, control.Model, target); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (service *Service) handleRouterShutdown(w http.ResponseWriter) {
	if service.shutdown == nil {
		openai.WriteError(w, http.StatusForbidden, "shutdown_disabled", "router shutdown is disabled")
		return
	}
	openai.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go service.shutdown()
}

func (service *Service) loadPublicModel(ctx context.Context, publicID string) error {
	publicID = strings.TrimSpace(publicID)
	if handled, err := service.loadRecipe(ctx, publicID); handled || err != nil {
		return err
	}
	if service.registry != nil && service.registry.HasModel(publicID) {
		model, ok := service.registry.Model(publicID)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		modelBackendMode, err := service.clusterModelBackendMode(model)
		if err != nil {
			return err
		}
		readiness := modelControlReadiness(modelBackendMode, model.HasEmbeddings, model.HasVoice, model.VLLMTask)
		route, release, ok := service.acquireRegistryModelControlRoute(ctx, publicID, modelBackendMode, readiness)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		defer release()
		if route.Remote {
			return service.clusterClient.Load(ctx, route.NodeURL, route.LocalID)
		}
		return service.loadLocalModel(ctx, route.PublicID, route.LocalID)
	}
	return service.loadLocalModel(ctx, publicID, publicID)
}

func (service *Service) unloadPublicModel(ctx context.Context, publicID string, target string) error {
	publicID = strings.TrimSpace(publicID)
	if publicID != "" && service.registry != nil && service.registry.HasModel(publicID) {
		model, ok := service.registry.Model(publicID)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		modelBackendMode, err := service.clusterModelBackendMode(model)
		if err != nil {
			return err
		}
		readiness := modelControlReadiness(modelBackendMode, model.HasEmbeddings, model.HasVoice, model.VLLMTask)
		route, release, ok := service.acquireRegistryModelControlRoute(ctx, publicID, modelBackendMode, readiness)
		if !ok {
			return fmt.Errorf("model %q was not found", publicID)
		}
		defer release()
		if route.Remote {
			return service.clusterClient.Unload(ctx, route.NodeURL, route.LocalID, target)
		}
	}
	return service.unloadLocal(ctx, target)
}

func (service *Service) loadLocalModel(ctx context.Context, publicID string, localID string) error {
	model, ok, err := service.catalog.Resolve(localID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("model %q was not found", publicID)
	}
	enabled, err := service.localModelEnabled(ctx, model.ID)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("model %q is disabled", publicID)
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return err
	}
	for _, readiness := range modelLoadReadinesses(modelBackendMode, model) {
		if err := service.loadLocalConfig(ctx, modelBackendMode, publicID, model.Filename, readiness); err != nil {
			return err
		}
	}
	return nil
}

func modelLoadReadinesses(mode string, model catalog.Model) []backendReadiness {
	if mode == BackendModeVLLM {
		return []backendReadiness{modelControlReadiness(mode, model.HasEmbeddings, model.HasVoice, model.VLLMTask)}
	}
	separateEmbeddings := model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate
	if mode != BackendModeLlamaSDCPP {
		readinesses := make([]backendReadiness, 0, 4)
		if modelHasExplicitTextAsset(model) || model.HasMultimodal || model.HasEmbeddings && !separateEmbeddings {
			readinesses = append(readinesses, readinessText)
		}
		if model.HasImage {
			readinesses = append(readinesses, readinessImage)
		}
		if model.HasVoice {
			readinesses = append(readinesses, readinessForVoiceModel(model, mode))
		}
		if model.HasMusic {
			readinesses = append(readinesses, readinessMusic)
		}
		if separateEmbeddings {
			readinesses = append(readinesses, readinessEmbeddings)
		}
		if len(readinesses) == 0 {
			return []backendReadiness{readinessText}
		}
		return readinesses
	}

	readinesses := make([]backendReadiness, 0, 3)
	if modelNeedsPrimaryTextRuntime(model) {
		readinesses = append(readinesses, readinessText)
	}
	if model.HasImage {
		readinesses = append(readinesses, readinessImage)
	}
	if separateEmbeddings {
		readinesses = append(readinesses, readinessEmbeddings)
	}
	if len(readinesses) == 0 {
		return []backendReadiness{readinessText}
	}
	return readinesses
}

func modelHasExplicitTextAsset(model catalog.Model) bool {
	for _, key := range []string{"model", "model_param", "draftmodel", "model_hash", "model_param_hash", "draftmodel_hash"} {
		value, found := model.Options[key]
		if !found {
			continue
		}
		var decoded any
		if json.Unmarshal(value, &decoded) != nil {
			continue
		}
		switch typed := decoded.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		case []any:
			if len(typed) > 0 {
				return true
			}
		}
	}
	return false
}

func modelControlReadiness(mode string, hasEmbeddings bool, hasVoice bool, task string) backendReadiness {
	if mode != BackendModeVLLM {
		if hasVoice {
			return readinessSpeech
		}
		return readinessText
	}
	if hasVoice || isVLLMSpeechTask(task) {
		return readinessTranscription
	}
	if hasEmbeddings {
		return readinessEmbeddings
	}
	return readinessText
}

func (service *Service) acquireRegistryModelControlRoute(ctx context.Context, publicID string, mode string, readiness backendReadiness) (cluster.Route, func(), bool) {
	healthy := service.localBackendAvailableForRoute(ctx, mode, readiness)
	switch readiness {
	case readinessEmbeddings:
		return service.registry.AcquireEmbedding(publicID, healthy)
	case readinessTranscription:
		return service.registry.AcquireVoice(publicID, healthy)
	default:
		return service.registry.Acquire(publicID, healthy)
	}
}

func (service *Service) loadLocalConfig(ctx context.Context, mode string, publicID string, filename string, readiness backendReadiness) error {
	_, release, _, err := service.acquireModelConfigForBackendMode(mode, ctx, publicID, filename, readiness, false)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (service *Service) loadLocalRuntimeForRequest(ctx context.Context, mode string, publicID string, filename string, readiness backendReadiness) error {
	modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(ctx), modelOperationTimeout)
	defer cancelModelContext()
	return service.loadLocalConfig(modelContext, mode, publicID, filename, readiness)
}

func (service *Service) unloadLocal(ctx context.Context, target string) error {
	family := service.backendFamilies[service.currentBackendMode()]
	if family == nil {
		return service.unloadSeparateLane(ctx, target)
	}
	runtimes, err := service.runtimesForUnloadTarget(family.mode, target)
	if err != nil {
		return err
	}
	if target == unloadpolicy.Embeddings {
		// Kobold and llama serve non-separate embeddings inline on the text runtime;
		// only a dedicated embeddings runtime (vLLM) and the pool should be unloaded.
		runtimes = distinctFromTextRuntime(family, runtimes)
	}
	if err := service.unloadRuntimes(ctx, runtimes); err != nil {
		return err
	}
	return service.unloadSeparateLane(ctx, target)
}

func distinctFromTextRuntime(family *backendFamily, runtimes []*backendRuntime) []*backendRuntime {
	filtered := make([]*backendRuntime, 0, len(runtimes))
	for _, runtime := range runtimes {
		if runtime != nil && runtime != family.textRuntime {
			filtered = append(filtered, runtime)
		}
	}
	return filtered
}

func readModelControlRequest(r *http.Request, requireModel bool) (modelControlRequest, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return modelControlRequest{}, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		if requireModel {
			return modelControlRequest{}, fmt.Errorf("model is required")
		}
		return modelControlRequest{}, nil
	}
	var control modelControlRequest
	if err := json.Unmarshal(body, &control); err != nil {
		return modelControlRequest{}, err
	}
	control.Model = strings.TrimSpace(control.Model)
	control.Target = strings.TrimSpace(control.Target)
	if requireModel && control.Model == "" {
		return modelControlRequest{}, fmt.Errorf("model is required")
	}
	return control, nil
}

func (service *Service) requireClusterToken(w http.ResponseWriter, r *http.Request) bool {
	if service.clusterToken == "" {
		openai.WriteError(w, http.StatusUnauthorized, "unauthorized", "cluster token is required")
		return false
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if !strings.HasPrefix(header, "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(service.clusterToken)) != 1 {
		openai.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid cluster token")
		return false
	}
	return true
}
