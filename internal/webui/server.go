package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/transportbody"
)

const (
	backendTicketQueryKey     = "tensors_router_ticket"
	maxWebUIControlBodyBytes  = 8 * transportbody.MiB
	maxWebUIProxyResponseSize = 32 * transportbody.GiB
)

type Server struct {
	config   Config
	router   *RouterProcess
	sessions *SessionManager
	client   *http.Client
	static   http.Handler
	assets   fs.FS
	access   *proxyAccessManager
}

type loginRequest struct {
	Token string `json:"token"`
}

func NewServer(config Config, router *RouterProcess, sessions *SessionManager) *Server {
	assets := AssetFS()
	return &Server{
		config:   config,
		router:   router,
		sessions: sessions,
		client:   &http.Client{Timeout: 0},
		static:   http.FileServer(http.FS(assets)),
		assets:   assets,
		access:   newProxyAccessManager(),
	}
}

func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server.serveAdminHTTP(w, r)
}

func (server *Server) AdminHandler() http.Handler {
	return http.HandlerFunc(server.serveAdminHTTP)
}

func (server *Server) BackendUIHandler() http.Handler {
	return http.HandlerFunc(server.serveBackendUIHTTP)
}

func (server *Server) serveAdminHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if limitAdminAPIRequestBody(w, r) {
			return
		}
		server.handleAPI(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/router/webuis/") {
		server.proxyRouterWebUI(w, r)
		return
	}
	if r.URL.Path == "/" {
		server.serveIndex(w)
		return
	}
	server.static.ServeHTTP(w, r)
}

func limitAdminAPIRequestBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || !stateChangingMethod(r.Method) {
		return false
	}
	if r.ContentLength > maxWebUIControlBodyBytes {
		writeWebError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return true
	}
	content, err := io.ReadAll(io.LimitReader(r.Body, maxWebUIControlBodyBytes+1))
	_ = r.Body.Close()
	if err != nil {
		writeWebError(w, http.StatusBadRequest, err.Error())
		return true
	}
	if int64(len(content)) > maxWebUIControlBodyBytes {
		writeWebError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(content))
	r.ContentLength = int64(len(content))
	return false
}

func (server *Server) serveBackendUIHTTP(w http.ResponseWriter, r *http.Request) {
	kind, path, ok := backendUIRouterPath(r)
	if !ok {
		writeWebError(w, http.StatusNotFound, "not found")
		return
	}
	if !server.authorizeBackendUIRequest(w, r, kind) {
		return
	}
	server.proxyRouterWebUIPath(w, r, path)
}

func (server *Server) serveIndex(w http.ResponseWriter) {
	content, err := fs.ReadFile(server.assets, "index.html")
	if err != nil {
		writeWebError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (server *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/login" && r.Method == http.MethodPost {
		server.handleLogin(w, r)
		return
	}
	if server.authenticationRequired() && !server.sessions.Authorized(r) {
		writeWebError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if server.authenticationRequired() && stateChangingMethod(r.Method) && !server.sessions.ValidCSRF(r) {
		writeWebError(w, http.StatusForbidden, "invalid csrf token")
		return
	}
	switch {
	case r.URL.Path == "/api/session" && r.Method == http.MethodGet:
		server.handleSession(w, r)
	case r.URL.Path == "/api/logout" && r.Method == http.MethodPost:
		server.sessions.Logout(w, r)
		writeWebJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.URL.Path == "/api/router/status" && r.Method == http.MethodGet:
		writeWebJSON(w, http.StatusOK, server.router.Status(r.Context()))
	case r.URL.Path == "/api/router/launch" && r.Method == http.MethodPost:
		server.handleRouterAction(w, r, "launch")
	case r.URL.Path == "/api/router/restart" && r.Method == http.MethodPost:
		server.handleRouterAction(w, r, "restart")
	case r.URL.Path == "/api/router/shutdown" && r.Method == http.MethodPost:
		server.handleRouterAction(w, r, "shutdown")
	case r.URL.Path == "/api/router/force-kill" && r.Method == http.MethodPost:
		server.handleRouterAction(w, r, "force-kill")
	case r.URL.Path == "/api/router/kill" && r.Method == http.MethodPost:
		server.handleRouterAction(w, r, "kill")
	case r.URL.Path == "/api/inventory" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/inventory")
	case r.URL.Path == "/api/nodes/state" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/nodes/state")
	case r.URL.Path == "/api/nodes/unload" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/nodes/unload")
	case r.URL.Path == "/api/nodes/backends/init" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/nodes/backends/init")
	case r.URL.Path == "/api/nodes/backends/init/cancel" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/nodes/backends/init/cancel")
	case r.URL.Path == "/api/nodes/backends/launch-options" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/nodes/backends/launch-options")
	case r.URL.Path == "/api/nodes/backends/launch-options" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/nodes/backends/launch-options")
	case r.URL.Path == "/api/models/state" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/models/state")
	case r.URL.Path == "/api/download/capabilities" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/download/capabilities")
	case r.URL.Path == "/api/download/search" && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/search")
	case r.URL.Path == "/api/download/search-page" && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/search-page")
	case r.URL.Path == "/api/download/repository" && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/repository")
	case r.URL.Path == "/api/download/plan" && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/plan")
	case r.URL.Path == "/api/download/jobs" && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/jobs")
	case strings.HasPrefix(r.URL.Path, "/api/download/jobs/") && r.Method == http.MethodGet:
		server.proxyDownload(w, r, http.MethodGet, "/router/v1/site/download/jobs/"+strings.TrimPrefix(r.URL.Path, "/api/download/jobs/"))
	case strings.HasPrefix(r.URL.Path, "/api/download/jobs/") && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/jobs/"+strings.TrimPrefix(r.URL.Path, "/api/download/jobs/"))
	case r.URL.Path == "/api/download/library" && r.Method == http.MethodGet:
		server.proxyDownload(w, r, http.MethodGet, "/router/v1/site/download/library")
	case r.URL.Path == "/api/download/rescan" && r.Method == http.MethodPost:
		server.proxyDownload(w, r, http.MethodPost, "/router/v1/site/download/rescan")
	case r.URL.Path == "/api/webuis" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/webuis")
	case r.URL.Path == "/api/webuis/session" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/webuis/session")
	case r.URL.Path == "/api/webuis/load" && r.Method == http.MethodPost:
		server.proxyWebUILoad(w, r)
	case r.URL.Path == "/api/benchmarks" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/benchmarks")
	case r.URL.Path == "/api/benchmarks/run" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/benchmarks/run")
	case r.URL.Path == "/api/analytics" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/analytics")
	case r.URL.Path == "/api/load-captures" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/load-captures")
	case strings.HasPrefix(r.URL.Path, "/api/load-captures/") && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/load-captures/"+strings.TrimPrefix(r.URL.Path, "/api/load-captures/"))
	case r.URL.Path == "/api/load" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/load")
	case r.URL.Path == "/api/cook/preview" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/cook/preview")
	case r.URL.Path == "/api/cook/apply" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/cook/apply")
	case strings.HasPrefix(r.URL.Path, "/api/cook/") && r.Method == http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/api/cook/")
		server.proxyRouter(w, r, http.MethodDelete, "/router/v1/site/cook/"+id)
	case r.URL.Path == "/api/config-file/preview" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/config-file/preview")
	case r.URL.Path == "/api/config-file/apply" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/config-file/apply")
	case r.URL.Path == "/api/config-file" && r.Method == http.MethodDelete:
		server.proxyRouter(w, r, http.MethodDelete, "/router/v1/site/config-file")
	case r.URL.Path == "/api/model-assets/export" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/export")
	case r.URL.Path == "/api/model-files/hash" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-files/hash")
	case r.URL.Path == "/api/model-assets/resolve" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/resolve")
	case r.URL.Path == "/api/model-assets/resolve-batch" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/resolve-batch")
	case r.URL.Path == "/api/model-assets/jobs" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/jobs")
	case r.URL.Path == "/api/model-assets/bind" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/bind")
	case r.URL.Path == "/api/model-assets/candidates" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/candidates")
	case r.URL.Path == "/api/model-assets/substitute" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/model-assets/substitute")
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/model-assets/jobs/"):
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/model-assets/jobs/"+strings.TrimPrefix(r.URL.Path, "/api/model-assets/jobs/"))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/model-assets/"):
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/model-assets/"+strings.TrimPrefix(r.URL.Path, "/api/model-assets/"))
	default:
		writeWebError(w, http.StatusNotFound, "not found")
	}
}

func (server *Server) proxyDownload(w http.ResponseWriter, r *http.Request, method string, path string) {
	server.proxyRouter(w, r, method, path)
}

func (server *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !server.authenticationRequired() {
		writeWebJSON(w, http.StatusOK, map[string]any{"authenticated": true, "csrf": ""})
		return
	}
	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeWebError(w, http.StatusBadRequest, err.Error())
		return
	}
	session, ok := server.sessions.Login(w, strings.TrimSpace(request.Token))
	if !ok {
		writeWebError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	writeWebJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrf":          session.CSRF,
	})
}

func (server *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if !server.authenticationRequired() {
		writeWebJSON(w, http.StatusOK, map[string]any{"authenticated": true, "csrf": ""})
		return
	}
	session, ok := server.sessions.Session(r)
	if !ok {
		writeWebError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeWebJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrf":          session.CSRF,
	})
}

func (server *Server) handleRouterAction(w http.ResponseWriter, r *http.Request, action string) {
	ctx := r.Context()
	var err error
	switch action {
	case "launch":
		err = server.router.Launch(ctx)
	case "restart":
		err = server.router.Restart(ctx)
	case "shutdown":
		err = server.router.GracefulShutdown(ctx)
	case "force-kill", "kill":
		err = server.router.ForceKill()
	}
	if err != nil {
		writeWebError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeWebJSON(w, http.StatusOK, server.router.Status(ctx))
}

func (server *Server) proxyRouter(w http.ResponseWriter, r *http.Request, method string, path string) {
	request, hasBody, ok := server.newRouterProxyRequest(w, r, method, path, false)
	if !ok {
		return
	}
	request.Header.Set("Accept", "application/json")
	if hasBody {
		request.Header.Set("Content-Type", "application/json")
	}
	server.forwardRouterProxyRequest(w, request, false)
}

func (server *Server) proxyWebUILoad(w http.ResponseWriter, r *http.Request) {
	request, hasBody, ok := server.newRouterProxyRequest(w, r, http.MethodPost, "/router/v1/site/webuis/load", false)
	if !ok {
		return
	}
	request.Header.Set("Accept", "application/json")
	if hasBody {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := server.client.Do(request)
	if err != nil {
		writeWebError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxWebUIControlBodyBytes+1))
	if err != nil || int64(len(content)) > maxWebUIControlBodyBytes {
		writeWebError(w, http.StatusBadGateway, "router response body too large")
		return
	}
	content = reportAndDiscardBackendDiagnostic(content)
	copyWebHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Length", "")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(content)
}

func reportAndDiscardBackendDiagnostic(content []byte) []byte {
	var response map[string]json.RawMessage
	if json.Unmarshal(content, &response) != nil {
		return content
	}
	rawDiagnostic, ok := response["backend_diagnostic"]
	if !ok {
		return content
	}
	var diagnostic backenddiagnostic.Diagnostic
	if json.Unmarshal(rawDiagnostic, &diagnostic) == nil {
		if diagnostic.ExitError != "" {
			log.Printf("backend load failure node=%s backend=%s exit=%s\n%s", diagnostic.NodeID, diagnostic.Backend, diagnostic.ExitError, diagnostic.Output)
		} else {
			log.Printf("backend load failure node=%s backend=%s\n%s", diagnostic.NodeID, diagnostic.Backend, diagnostic.Output)
		}
	}
	delete(response, "backend_diagnostic")
	filtered, err := json.Marshal(response)
	if err != nil {
		return content
	}
	return filtered
}

func (server *Server) proxyRouterWebUI(w http.ResponseWriter, r *http.Request) {
	kind, ok := webUIKindFromPath(r.URL.Path)
	if !ok {
		writeWebError(w, http.StatusNotFound, "not found")
		return
	}
	if server.authenticationRequired() && !server.sessions.Authorized(r) {
		writeWebError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	target, err := server.backendUIURL(r, kind)
	if err != nil {
		writeWebError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (server *Server) proxyRouterWebUIPath(w http.ResponseWriter, r *http.Request, path string) {
	request, _, ok := server.newRouterProxyRequest(w, r, r.Method, path, true)
	if !ok {
		return
	}
	copyRouterWebUIHeaders(request.Header, r.Header)
	server.forwardRouterProxyRequest(w, request, true)
}

func (server *Server) backendUIURL(r *http.Request, kind string) (string, error) {
	origin, err := server.backendUIOrigin(r)
	if err != nil {
		return "", err
	}
	target, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	target.Path = r.URL.Path
	query := r.URL.Query()
	if server.authenticationRequired() {
		query.Set(backendTicketQueryKey, server.access.Issue(kind))
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (server *Server) backendUIOrigin(r *http.Request) (string, error) {
	if configured := strings.TrimSpace(server.config.Server.BackendUIPublicURL); configured != "" {
		return strings.TrimRight(configured, "/"), nil
	}
	_, port, err := net.SplitHostPort(NormalizeBind(server.config.Server.BackendUIBind))
	if err != nil {
		return "", err
	}
	host := requestHostname(r.Host)
	if host == "" {
		return "", &net.AddrError{Err: "missing request host", Addr: r.Host}
	}
	return "https://" + net.JoinHostPort(host, port), nil
}

func requestHostname(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return host
	}
	return strings.Trim(hostPort, "[]")
}

func (server *Server) authorizeBackendUIRequest(w http.ResponseWriter, r *http.Request, kind string) bool {
	if !server.authenticationRequired() {
		return true
	}
	ticket := strings.TrimSpace(r.URL.Query().Get(backendTicketQueryKey))
	if ticket != "" {
		query := r.URL.Query()
		query.Del(backendTicketQueryKey)
		target, ok := localRedirectTarget(r.URL, query)
		if !ok {
			writeWebError(w, http.StatusBadRequest, "invalid backend UI redirect path")
			return false
		}
		if r.Method != http.MethodGet || !server.access.Exchange(w, ticket, kind) {
			writeWebError(w, http.StatusUnauthorized, "invalid or expired backend UI ticket")
			return false
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusFound)
		return false
	}
	if !server.access.Authorized(r, kind) {
		writeWebError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func localRedirectTarget(requestURL *url.URL, query url.Values) (string, bool) {
	if requestURL == nil || !isLocalRedirectPath(requestURL.Path) || !isLocalRedirectPath(requestURL.EscapedPath()) {
		return "", false
	}
	target := &url.URL{Path: requestURL.Path, RawPath: requestURL.RawPath, ForceQuery: requestURL.ForceQuery, RawQuery: query.Encode()}
	return target.String(), true
}

func isLocalRedirectPath(path string) bool {
	return len(path) > 0 && path[0] == '/' && (len(path) == 1 || (path[1] != '/' && path[1] != '\\'))
}

func (server *Server) authenticationRequired() bool {
	return server.config.Security.Profile != SecurityProfileTrustedLAN
}

func (server *Server) newRouterProxyRequest(w http.ResponseWriter, r *http.Request, method string, path string, streamBody bool) (*http.Request, bool, bool) {
	if err := server.router.EnsureStarted(r.Context()); err != nil {
		writeWebError(w, http.StatusBadGateway, err.Error())
		return nil, false, false
	}
	var body io.Reader
	hasBody := false
	if r.Body != nil {
		if streamBody {
			body = r.Body
			hasBody = r.ContentLength != 0
		} else {
			content, err := io.ReadAll(io.LimitReader(r.Body, maxWebUIControlBodyBytes+1))
			_ = r.Body.Close()
			if err != nil {
				writeWebError(w, http.StatusBadRequest, err.Error())
				return nil, false, false
			}
			if int64(len(content)) > maxWebUIControlBodyBytes {
				writeWebError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return nil, false, false
			}
			hasBody = len(content) > 0
			body = bytes.NewReader(content)
		}
	}
	target := strings.TrimRight(server.router.URL(), "/") + path
	if strings.TrimSpace(r.URL.RawQuery) != "" {
		target += "?" + r.URL.RawQuery
	}
	request, err := http.NewRequestWithContext(r.Context(), method, target, body)
	if err != nil {
		writeWebError(w, http.StatusBadRequest, err.Error())
		return nil, false, false
	}
	if streamBody {
		request.ContentLength = r.ContentLength
	}
	if token := strings.TrimSpace(server.config.Router.Token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request, hasBody, true
}

func (server *Server) forwardRouterProxyRequest(w http.ResponseWriter, request *http.Request, preserveRedirects bool) {
	client := server.client
	if preserveRedirects {
		copiedClient := *server.client
		copiedClient.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &copiedClient
	}
	response, err := client.Do(request)
	if err != nil {
		writeWebError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer response.Body.Close()
	if response.ContentLength > maxWebUIProxyResponseSize {
		writeWebError(w, http.StatusBadGateway, "router response body too large")
		return
	}
	copyWebHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = transportbody.CopyResponse(webFlushingWriter{ResponseWriter: w}, response.Body, maxWebUIProxyResponseSize)
}

type webFlushingWriter struct {
	http.ResponseWriter
}

func (writer webFlushingWriter) Write(content []byte) (int, error) {
	written, err := writer.ResponseWriter.Write(content)
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	return written, err
}

func stateChangingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func writeWebJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeWebError(w http.ResponseWriter, status int, message string) {
	writeWebJSON(w, status, map[string]any{"error": message})
}

func copyWebHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") || isWebHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	dst.Set("X-Content-Type-Options", "nosniff")
}

func copyRouterWebUIHeaders(dst http.Header, src http.Header) {
	blocked := webConnectionHeaderNames(src)
	for key, values := range src {
		if _, connected := blocked[strings.ToLower(key)]; connected || skipRouterWebUIHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func skipRouterWebUIHeader(key string) bool {
	if isWebHopByHopHeader(key) {
		return true
	}
	switch strings.ToLower(key) {
	case "authorization", "cookie", "forwarded", "proxy-connection", "x-real-ip":
		return true
	default:
		lower := strings.ToLower(key)
		return strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "proxy-")
	}
}

func webConnectionHeaderNames(header http.Header) map[string]struct{} {
	blocked := map[string]struct{}{}
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
				blocked[name] = struct{}{}
			}
		}
	}
	return blocked
}

func webUIBackendProxyPath(r *http.Request) (string, bool) {
	kind, ok := webUIBackendProxyKind(r)
	if !ok {
		return "", false
	}
	return "/router/webuis/" + kind + "/" + strings.TrimLeft(r.URL.Path, "/"), true
}

func backendUIRouterPath(r *http.Request) (string, string, bool) {
	if kind, ok := webUIKindFromPath(r.URL.Path); ok {
		return kind, r.URL.Path, true
	}
	kind, ok := webUIBackendProxyKind(r)
	if !ok {
		return "", "", false
	}
	path, ok := webUIBackendProxyPath(r)
	return kind, path, ok
}

func webUIBackendProxyKind(r *http.Request) (string, bool) {
	if kind, ok := webUIKindFromReferer(r); ok && webUIBackendPathAllowed(kind, r.URL.Path) {
		return kind, true
	}
	if strings.HasPrefix(r.URL.Path, "/sdcpp/v1/") {
		return "sdcpp", true
	}
	return "", false
}

func webUIKindFromReferer(r *http.Request) (string, bool) {
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer == "" {
		return "", false
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		return "", false
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, r.Host) {
		return "", false
	}
	return webUIKindFromPath(parsed.Path)
}

func webUIKindFromPath(path string) (string, bool) {
	const prefix = "/router/webuis/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	remainder := strings.TrimPrefix(path, prefix)
	kind, _, _ := strings.Cut(remainder, "/")
	if webUIKindKnown(kind) {
		return kind, true
	}
	return "", false
}

func webUIKindKnown(kind string) bool {
	switch kind {
	case "kobold-lite", "kobold-lcpp", "kobold-sd", "kobold-music", "llama", "sdcpp":
		return true
	default:
		return false
	}
}

func webUIBackendPathAllowed(kind string, path string) bool {
	switch kind {
	case "sdcpp":
		return webUIPathHasPrefix(path, "/sdcpp/v1/", "/sdapi/v1/", "/v1/images/") || path == "/v1/models"
	case "llama":
		return webUIPathHasPrefix(path, "/v1/", "/api/v1/") ||
			webUIPathIs(path, "/completion", "/chat", "/infill", "/embedding", "/embeddings", "/rerank", "/tokenize", "/detokenize", "/props", "/slots", "/metrics", "/health")
	case "kobold-lite", "kobold-lcpp":
		return webUIPathHasPrefix(path, "/v1/", "/api/v1/", "/api/extra/") ||
			webUIPathIs(path, "/api/generate", "/api/chat", "/api/show", "/api/tags", "/api/ps", "/api/version")
	case "kobold-sd":
		return webUIPathHasPrefix(path, "/sdapi/v1/", "/v1/images/", "/history/", "/view/", "/object_info/", "/upload/image") ||
			webUIPathIs(path, "/prompt", "/queue", "/history", "/view", "/object_info", "/system_stats", "/interrupt")
	case "kobold-music":
		return webUIPathHasPrefix(path, "/api/extra/music/")
	default:
		return false
	}
}

func webUIPathHasPrefix(path string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) || strings.TrimRight(prefix, "/") == path {
			return true
		}
	}
	return false
}

func webUIPathIs(path string, values ...string) bool {
	for _, value := range values {
		if path == value {
			return true
		}
	}
	return false
}

func isWebHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func WebHTTPServer(bind string, handler http.Handler) *http.Server {
	// Normalize bind to a TCP address. Accept values like
	// "0.0.0.0:8443" or with a scheme like "https://0.0.0.0:8443" and
	// strip any scheme so net.Listen gets a valid host:port.
	addr := NormalizeBind(bind)
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func NormalizeBind(bind string) string {
	b := strings.TrimSpace(bind)
	b = strings.TrimSuffix(b, "/")
	if strings.HasPrefix(b, "http://") {
		return strings.TrimPrefix(b, "http://")
	}
	if strings.HasPrefix(b, "https://") {
		return strings.TrimPrefix(b, "https://")
	}
	return b
}
