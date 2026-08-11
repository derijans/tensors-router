package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultRequestBodyBytes  = int64(8 * 1024 * 1024)
	defaultResponseBodyBytes = int64(8 * 1024 * 1024)
	defaultRequestTimeout    = 3 * time.Minute
)

type GatewayConfig struct {
	MaxRequestBodyBytes  int64
	MaxResponseBodyBytes int64
	RequestTimeout       time.Duration
}

type Gateway struct {
	client               *http.Client
	maxRequestBodyBytes  int64
	maxResponseBodyBytes int64
	requestTimeout       time.Duration
	backendRequestID     atomic.Uint64
}

type Target struct {
	Backend string
	URL     *url.URL
}

type backendAdapter interface {
	ListTools(context.Context) ([]*mcpsdk.Tool, error)
	CallTool(context.Context, string, json.RawMessage) (*mcpsdk.CallToolResult, error)
}

func NewGateway(config GatewayConfig) *Gateway {
	if config.MaxRequestBodyBytes <= 0 {
		config.MaxRequestBodyBytes = defaultRequestBodyBytes
	}
	if config.MaxResponseBodyBytes <= 0 {
		config.MaxResponseBodyBytes = defaultResponseBodyBytes
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaultRequestTimeout
	}
	return &Gateway{
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fmt.Errorf("backend redirects are not allowed")
			},
		},
		maxRequestBodyBytes:  config.MaxRequestBodyBytes,
		maxResponseBodyBytes: config.MaxResponseBodyBytes,
		requestTimeout:       config.RequestTimeout,
	}
}

func (gateway *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request, target Target) error {
	if gateway == nil {
		return fmt.Errorf("MCP gateway is disabled")
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil
	}
	adapter, err := gateway.adapter(target)
	if err != nil {
		return err
	}
	requestContext, cancel := context.WithTimeout(r.Context(), gateway.requestTimeout)
	defer cancel()
	tools, err := adapter.ListTools(requestContext)
	if err != nil {
		return err
	}
	server, err := gateway.server(requestContext, adapter, tools)
	if err != nil {
		return err
	}
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, &mcpsdk.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		DisableLocalhostProtection:   true,
		MaxRequestBodyBytes:          gateway.maxRequestBodyBytes,
		PropagateRequestCancellation: true,
	})
	response := newBoundedResponseWriter(gateway.maxResponseBodyBytes)
	handler.ServeHTTP(response, r.WithContext(requestContext))
	if response.overflowed {
		return fmt.Errorf("MCP response exceeds %d bytes", gateway.maxResponseBodyBytes)
	}
	return response.writeTo(w)
}

func (gateway *Gateway) adapter(target Target) (backendAdapter, error) {
	if target.URL == nil {
		return nil, fmt.Errorf("backend URL is required")
	}
	client := &backendClient{gateway: gateway, target: Target{Backend: target.Backend, URL: cloneURL(target.URL)}}
	switch target.Backend {
	case BackendKobold:
		return &koboldAdapter{backendClient: client}, nil
	case BackendLlama:
		return &llamaAdapter{backendClient: client}, nil
	default:
		return nil, fmt.Errorf("backend mode %q does not support MCP", target.Backend)
	}
}

func (gateway *Gateway) server(requestContext context.Context, adapter backendAdapter, tools []*mcpsdk.Tool) (*mcpsdk.Server, error) {
	for _, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("backend returned an MCP tool without a name")
		}
	}
	sort.Slice(tools, func(left int, right int) bool {
		return tools[left].Name < tools[right].Name
	})
	seen := make(map[string]struct{}, len(tools))
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "tensors-router", Version: "1"}, &mcpsdk.ServerOptions{
		Capabilities: &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{}},
	})
	for _, tool := range tools {
		if _, exists := seen[tool.Name]; exists {
			return nil, fmt.Errorf("backend returned duplicate MCP tool %q", tool.Name)
		}
		inputSchema, err := objectInputSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("MCP tool %q: %w", tool.Name, err)
		}
		tool.InputSchema = inputSchema
		seen[tool.Name] = struct{}{}
		toolName := tool.Name
		handler := func(handlerContext context.Context, request *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			if request == nil || request.Params == nil {
				return nil, fmt.Errorf("MCP tool call parameters are required")
			}
			callContext, cancel := context.WithCancel(requestContext)
			stopCancellation := context.AfterFunc(handlerContext, cancel)
			defer stopCancellation()
			defer cancel()
			return adapter.CallTool(callContext, toolName, request.Params.Arguments)
		}
		if err := addTool(server, tool, handler); err != nil {
			return nil, fmt.Errorf("MCP tool %q: %w", tool.Name, err)
		}
	}
	return server, nil
}

func addTool(server *mcpsdk.Server, tool *mcpsdk.Tool, handler mcpsdk.ToolHandler) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool definition is invalid: %v", recovered)
		}
	}()
	server.AddTool(tool, handler)
	return nil
}

func (gateway *Gateway) nextBackendRequestID() uint64 {
	return gateway.backendRequestID.Add(1)
}

func objectInputSchema(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage(`{"type":"object"}`), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("input schema is invalid: %w", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return nil, fmt.Errorf("input schema is not an object")
	}
	if schemaType, exists := schema["type"]; exists && schemaType != "object" {
		return nil, fmt.Errorf("input schema type must be object")
	}
	schema["type"] = "object"
	return json.Marshal(schema)
}

func cloneURL(value *url.URL) *url.URL {
	copyValue := *value
	return &copyValue
}
