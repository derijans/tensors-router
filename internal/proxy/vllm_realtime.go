package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleVLLMRealtime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !headerContainsToken(r.Header, "Connection", "upgrade") || !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Realtime requires a WebSocket upgrade")
		return
	}
	modelID := strings.TrimSpace(r.URL.Query().Get("model"))
	if modelID == "" {
		modelID = strings.TrimSpace(r.Header.Get("X-Tensors-Model"))
	}
	if modelID == "" {
		openai.WriteError(w, http.StatusBadRequest, "streaming_model_selector_required", "model selector is required")
		return
	}
	model, route, release, ok := service.acquireRealtimeRoute(r, modelID)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", modelID))
		return
	}
	defer release()
	if !clusterModelSupportsSTT(model) {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
		return
	}
	backendModelID := vllmRequestModelID(modelID, route.LocalID, model.ServedNames)
	response, runtimeRelease, err := service.openRealtimeBackend(r, route, backendModelID)
	if err != nil {
		writeBackendFailure(w, err)
		return
	}
	defer runtimeRelease()
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("backend returned status %d", response.StatusCode))
		return
	}
	backendStream, ok := response.Body.(io.ReadWriteCloser)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", "backend did not return a bidirectional WebSocket stream")
		return
	}
	controller := http.NewResponseController(w)
	clientConnection, clientBuffer, err := controller.Hijack()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "upgrade_failed", "WebSocket hijacking is unavailable")
		return
	}
	defer clientConnection.Close()
	if err := writeRealtimeSwitchingProtocols(clientBuffer, response); err != nil {
		return
	}
	proxyBidirectionalStream(r.Context(), clientConnection, backendStream, service.transportLimits.MaxRequestBytes, service.transportLimits.MaxResponseBytes)
}

func (service *Service) acquireRealtimeRoute(r *http.Request, publicID string) (cluster.Model, cluster.Route, func(), bool) {
	if service.registry != nil {
		model, ok := service.registry.VoiceModel(publicID)
		if !ok {
			model, ok = registryVLLMVoiceModelForServedName(service.registry.Models(), publicID)
			if !ok {
				return cluster.Model{}, cluster.Route{}, func() {}, false
			}
		}
		mode, err := service.clusterModelBackendMode(model)
		if err != nil || mode != BackendModeVLLM {
			return cluster.Model{}, cluster.Route{}, func() {}, false
		}
		route, release, acquired := service.registry.AcquireVoice(model.PublicID, service.localBackendAvailableForRoute(r.Context(), mode, readinessTranscription))
		return model, route, release, acquired
	}
	model, ok, err := service.catalog.Resolve(publicID)
	if err != nil || !ok || !catalogModelSupportsSTT(model) {
		return cluster.Model{}, cluster.Route{}, func() {}, false
	}
	mode, err := service.catalogModelBackendMode(model)
	if err != nil || mode != BackendModeVLLM {
		return cluster.Model{}, cluster.Route{}, func() {}, false
	}
	return cluster.Model{PublicID: publicID, LocalID: model.ID, Filename: model.Filename, BackendMode: mode, HasVoice: true, ServedNames: append([]string{}, model.ServedNames...), VLLMTask: model.VLLMTask}, cluster.Route{PublicID: publicID, LocalID: model.ID, Filename: model.Filename, BackendMode: mode}, func() {}, true
}

func (service *Service) openRealtimeBackend(r *http.Request, route cluster.Route, backendModelID string) (*http.Response, func(), error) {
	if route.Remote {
		response, err := service.openRemoteRealtime(r, route, backendModelID)
		return response, func() {}, err
	}
	modelContext, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	runtime, release, _, err := service.acquireModelConfigForBackendMode(BackendModeVLLM, modelContext, route.LocalID, route.Filename, readinessTranscription, false)
	cancel()
	if err != nil {
		return nil, func() {}, err
	}
	target := runtime.backend.URL()
	target.Path = joinPath(target.Path, "/v1/realtime")
	values := r.URL.Query()
	values.Set("model", backendModelID)
	target.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		release()
		return nil, func() {}, err
	}
	copyRealtimeHeaders(request.Header, r.Header)
	request.Host = target.Host
	response, err := service.backendHTTPClient(runtime.backend).Do(request)
	if err != nil {
		release()
		return nil, func() {}, err
	}
	return response, release, nil
}

func (service *Service) openRemoteRealtime(r *http.Request, route cluster.Route, backendModelID string) (*http.Response, error) {
	baseURL, err := service.clusterClient.AuthorizedBaseURL(route.NodeURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	target.Path = joinPath(target.Path, "/router/v1/node/inference/v1/realtime")
	values := r.URL.Query()
	values.Set("model", backendModelID)
	target.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	copyRealtimeHeaders(request.Header, r.Header)
	request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	request.Host = target.Host
	client := *service.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client.Do(request)
}

func registryVLLMVoiceModelForServedName(models []cluster.Model, servedName string) (cluster.Model, bool) {
	var selected cluster.Model
	identity := ""
	for _, model := range models {
		if model.Disabled || model.BackendMode != BackendModeVLLM || !model.HasVoice || !servedNameIn(servedName, model.ServedNames) {
			continue
		}
		candidateIdentity := firstNonEmpty(model.ConfigHash, model.Filename+"\x00"+model.LocalID)
		if identity != "" && identity != candidateIdentity {
			return cluster.Model{}, false
		}
		identity = candidateIdentity
		selected = model
	}
	return selected, identity != ""
}

func servedNameIn(name string, servedNames []string) bool {
	for _, candidate := range servedNames {
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

func copyRealtimeHeaders(destination http.Header, source http.Header) {
	for _, name := range []string{"Connection", "Upgrade", "Origin", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions", "User-Agent", "X-Request-ID"} {
		for _, value := range source.Values(name) {
			destination.Add(name, value)
		}
	}
}

func headerContainsToken(header http.Header, name string, token string) bool {
	for _, value := range header.Values(name) {
		for _, candidate := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(candidate), token) {
				return true
			}
		}
	}
	return false
}

func writeRealtimeSwitchingProtocols(writer *bufio.ReadWriter, response *http.Response) error {
	if _, err := writer.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		return err
	}
	for _, name := range []string{"Connection", "Upgrade", "Sec-WebSocket-Accept", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"} {
		for _, value := range response.Header.Values(name) {
			if _, err := writer.WriteString(name + ": " + value + "\r\n"); err != nil {
				return err
			}
		}
	}
	if _, err := writer.WriteString("\r\n"); err != nil {
		return err
	}
	return writer.Flush()
}

func proxyBidirectionalStream(ctx context.Context, client io.ReadWriteCloser, backend io.ReadWriteCloser, maximumClientBytes int64, maximumBackendBytes int64) {
	var wait sync.WaitGroup
	wait.Add(2)
	copyStream := func(destination io.Writer, source io.Reader, maximumBytes int64) {
		defer wait.Done()
		_, _ = io.Copy(destination, io.LimitReader(source, maximumBytes))
		_ = client.Close()
		_ = backend.Close()
	}
	go copyStream(backend, client, maximumClientBytes)
	go copyStream(client, backend, maximumBackendBytes)
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		_ = client.Close()
		_ = backend.Close()
		<-done
	case <-done:
	}
}
