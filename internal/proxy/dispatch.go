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
	if r.URL.Path == "/v1/realtime" {
		service.handleVLLMRealtime(w, r)
		return
	}
	if _, _, responseOperation := vllmResponseOperation(r.URL.Path); responseOperation {
		service.handleVLLMResponseOperation(w, r)
		return
	}
	if rejectOllamaMethod(w, r) {
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
	if service.prepareOrHandleTransport(w, r, workingSet) {
		return
	}
	if service.limitControlRequestBody(w, r) {
		return
	}
	if r.URL.Path == "/router/mcp" && service.mcpGateway != nil {
		service.handlePublicMCP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/router/v1/") {
		service.routes.serve(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, siteWebUIProxyPrefix) {
		service.webUI.handleSiteWebUIProxy(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/api/admin/") {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}

	if isOllamaPath(r.URL.Path) {
		service.handleOllamaRequest(w, r)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		service.handleModels(w)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/ping" {
		openai.WriteJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
		service.handleImageModels(w)
		return
	}

	if r.URL.Path == "/sdapi/v1/options" {
		service.handleImageOptions(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/sdapi/v1/refresh-checkpoints" {
		openai.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if isVoicePath(r.URL.Path) || isMusicPath(r.URL.Path) {
		service.handleAudioRequest(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/prompt" {
		service.handleComfyPrompt(w, r)
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/history/") {
		if service.handleComfyHistory(w, r) {
			return
		}
	}
	if r.Method == http.MethodGet && r.URL.Path == "/view" {
		if service.handleComfyView(w, r) {
			return
		}
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload/image") {
		if service.handleComfyUploadImage(w, r) {
			return
		}
	}

	if isImagePath(r.URL.Path) {
		service.handleImageRequest(w, r)
		return
	}

	if isTextPath(r.URL.Path) {
		service.handleModelRequest(w, r, textPathRequiresModel(r.URL.Path))
		return
	}

	openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
}
