package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type koboldAdapter struct {
	*backendClient
}

func (adapter *koboldAdapter) ListTools(ctx context.Context) ([]*mcpsdk.Tool, error) {
	result, err := adapter.rpc(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var list mcpsdk.ListToolsResult
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("KoboldCpp returned an invalid MCP tool list: %w", err)
	}
	return list.Tools, nil
}

func (adapter *koboldAdapter) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*mcpsdk.CallToolResult, error) {
	params := map[string]any{"name": name}
	if len(arguments) > 0 {
		params["arguments"] = arguments
	}
	result, err := adapter.rpc(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var callResult mcpsdk.CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("KoboldCpp returned an invalid MCP tool result: %w", err)
	}
	return &callResult, nil
}

func (adapter *koboldAdapter) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      adapter.gateway.nextBackendRequestID(),
		"method":  method,
		"params":  params,
	}
	content, err := adapter.request(ctx, http.MethodPost, "/mcp", payload)
	if err != nil {
		return nil, err
	}
	var response rpcResponse
	if err := json.Unmarshal(content, &response); err != nil {
		return nil, fmt.Errorf("KoboldCpp returned invalid MCP JSON: %w", err)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("KoboldCpp MCP error %d: %s", response.Error.Code, response.Error.Message)
	}
	if len(response.Result) == 0 {
		return nil, fmt.Errorf("KoboldCpp returned no MCP result")
	}
	return response.Result, nil
}
