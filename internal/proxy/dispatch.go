package proxy

import (
	"net/http"
	"strings"

	"tensors-router/internal/ollama"
	"tensors-router/internal/openai"
	"tensors-router/internal/transportbody"
)

func (service *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isOllamaPath(r.URL.Path) {
		service.serveHTTP(w, r)
		return
	}
	writer := ollama.NewErrorResponseWriter(w)
	service.serveHTTP(writer, r)
	writer.Finish()
}
func (service *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if service.serveVLLMSessionPaths(w, r) {
		return
	}
	if rejectOllamaMethod(w, r) || rejectOpenAIMethod(w, r) {
		return
	}
	workingSet, ok := service.reserveTransportWorkingSet(r)
	if !ok {
		writeTransportError(w, transportbody.ErrBufferCapacity)
		return
	}
	if workingSet != nil {
		defer workingSet.Release()
	}
	if service.prepareOrHandleTransport(w, r, workingSet) || service.limitControlRequestBody(w, r) {
		return
	}
	if service.serveRouterPaths(w, r) || service.serveCompatibilityPaths(w, r) || service.serveCapabilityLanes(w, r) {
		return
	}
	openai.WriteEndpointNotFound(w)
}

func (service *Service) serveVLLMSessionPaths(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/v1/realtime" {
		service.handleVLLMRealtime(w, r)
		return true
	}
	if _, _, responseOperation := vllmResponseOperation(r.URL.Path); responseOperation {
		service.handleVLLMResponseOperation(w, r)
		return true
	}
	return false
}

func (service *Service) serveRouterPaths(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	switch {
	case path == "/router/mcp" && service.mcpGateway != nil:
		service.handlePublicMCP(w, r)
	case strings.HasPrefix(path, "/router/v1/"):
		service.routes.serve(w, r)
	case strings.HasPrefix(path, siteWebUIProxyPrefix):
		service.webUI.handleSiteWebUIProxy(w, r)
	case strings.HasPrefix(path, "/api/admin/"):
		openai.WriteEndpointNotFound(w)
	default:
		return false
	}
	return true
}

func (service *Service) serveCompatibilityPaths(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	switch {
	case isOllamaPath(path):
		service.handleOllamaRequest(w, r)
	case r.Method == http.MethodGet && path == "/v1/models":
		service.handleModels(w)
	case r.Method == http.MethodGet && path == "/ping":
		openai.WriteJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	case r.Method == http.MethodGet && path == "/sdapi/v1/sd-models":
		service.handleImageModels(w)
	case path == "/sdapi/v1/options":
		service.handleImageOptions(w, r)
	case r.Method == http.MethodPost && path == "/sdapi/v1/refresh-checkpoints":
		openai.WriteJSON(w, http.StatusOK, map[string]any{})
	default:
		return false
	}
	return true
}

func (service *Service) serveCapabilityLanes(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	switch {
	case isVoicePath(path) || isMusicPath(path):
		service.handleAudioRequest(w, r)
		return true
	case service.serveComfyPaths(w, r):
		return true
	case isImagePath(path):
		service.handleImageRequest(w, r)
		return true
	case isTextPath(path):
		service.handleModelRequest(w, r, textPathRequiresModel(path))
		return true
	default:
		return false
	}
}

func (service *Service) serveComfyPaths(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && path == "/prompt":
		service.handleComfyPrompt(w, r)
		return true
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/history/"):
		return service.handleComfyHistory(w, r)
	case r.Method == http.MethodGet && path == "/view":
		return service.handleComfyView(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/upload/image"):
		return service.handleComfyUploadImage(w, r)
	default:
		return false
	}
}
