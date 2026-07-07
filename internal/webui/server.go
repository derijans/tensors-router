package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Server struct {
	config   Config
	router   *RouterProcess
	sessions *SessionManager
	client   *http.Client
	static   http.Handler
	assets   fs.FS
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
	}
}

func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if path, ok := webUIBackendProxyPath(r); ok {
		server.proxyRouterWebUIPath(w, r, path)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
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
	if !server.sessions.Authorized(r) {
		writeWebError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if stateChangingMethod(r.Method) && !server.sessions.ValidCSRF(r) {
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
	case r.URL.Path == "/api/webuis" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/webuis")
	case r.URL.Path == "/api/webuis/session" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/webuis/session")
	case r.URL.Path == "/api/webuis/load" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/site/webuis/load")
	case r.URL.Path == "/api/benchmarks" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/benchmarks")
	case r.URL.Path == "/api/benchmarks/run" && r.Method == http.MethodPost:
		server.proxyRouter(w, r, http.MethodPost, "/router/v1/benchmarks/run")
	case r.URL.Path == "/api/analytics" && r.Method == http.MethodGet:
		server.proxyRouter(w, r, http.MethodGet, "/router/v1/site/analytics")
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
	default:
		writeWebError(w, http.StatusNotFound, "not found")
	}
}

func (server *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
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
	request, hasBody, ok := server.newRouterProxyRequest(w, r, method, path)
	if !ok {
		return
	}
	request.Header.Set("Accept", "application/json")
	if hasBody {
		request.Header.Set("Content-Type", "application/json")
	}
	server.forwardRouterProxyRequest(w, request, false)
}

func (server *Server) proxyRouterWebUI(w http.ResponseWriter, r *http.Request) {
	server.proxyRouterWebUIPath(w, r, r.URL.Path)
}

func (server *Server) proxyRouterWebUIPath(w http.ResponseWriter, r *http.Request, path string) {
	if !server.sessions.Authorized(r) {
		writeWebError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	request, _, ok := server.newRouterProxyRequest(w, r, r.Method, path)
	if !ok {
		return
	}
	copyRouterWebUIHeaders(request.Header, r.Header)
	server.forwardRouterProxyRequest(w, request, true)
}

func (server *Server) newRouterProxyRequest(w http.ResponseWriter, r *http.Request, method string, path string) (*http.Request, bool, bool) {
	if err := server.router.EnsureStarted(r.Context()); err != nil {
		writeWebError(w, http.StatusBadGateway, err.Error())
		return nil, false, false
	}
	var body io.Reader
	hasBody := false
	if r.Body != nil {
		content, err := io.ReadAll(r.Body)
		if err != nil {
			writeWebError(w, http.StatusBadRequest, err.Error())
			return nil, false, false
		}
		hasBody = len(content) > 0
		body = bytes.NewReader(content)
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
	copyWebHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
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
}

func copyRouterWebUIHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if skipRouterWebUIHeader(key) {
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
	case "authorization", "cookie":
		return true
	default:
		return false
	}
}

func webUIBackendProxyPath(r *http.Request) (string, bool) {
	kind, ok := webUIBackendProxyKind(r)
	if !ok {
		return "", false
	}
	return "/router/webuis/" + kind + "/" + strings.TrimLeft(r.URL.Path, "/"), true
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
