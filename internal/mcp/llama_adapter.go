package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type llamaTool struct {
	Tool       string              `json:"tool"`
	Definition llamaToolDefinition `json:"definition"`
}

type llamaToolDefinition struct {
	Function llamaFunction `json:"function"`
}

type llamaFunction struct {
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type llamaAdapter struct {
	*backendClient
}

func (adapter *llamaAdapter) ListTools(ctx context.Context) ([]*mcpsdk.Tool, error) {
	content, err := adapter.request(ctx, http.MethodGet, "/tools", nil)
	if err != nil {
		return nil, err
	}
	var nativeTools []llamaTool
	if err := json.Unmarshal(content, &nativeTools); err != nil {
		return nil, fmt.Errorf("llama.cpp returned an invalid tool list: %w", err)
	}
	tools := make([]*mcpsdk.Tool, 0, len(nativeTools))
	for _, nativeTool := range nativeTools {
		name := nativeTool.Tool
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("llama.cpp returned a tool without a name")
		}
		var schema any
		if len(nativeTool.Definition.Function.Parameters) > 0 && string(nativeTool.Definition.Function.Parameters) != "null" {
			if err := json.Unmarshal(nativeTool.Definition.Function.Parameters, &schema); err != nil {
				return nil, fmt.Errorf("llama.cpp tool %q returned an invalid input schema: %w", name, err)
			}
		}
		tools = append(tools, &mcpsdk.Tool{
			Name:        name,
			Description: nativeTool.Definition.Function.Description,
			InputSchema: schema,
		})
	}
	return tools, nil
}

func (adapter *llamaAdapter) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*mcpsdk.CallToolResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	payload := map[string]any{"tool": name, "params": arguments}
	content, err := adapter.request(ctx, http.MethodPost, "/tools", payload)
	if err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, fmt.Errorf("llama.cpp returned an invalid tool result: %w", err)
	}
	if rawError, exists := envelope["error"]; exists {
		var message string
		if err := json.Unmarshal(rawError, &message); err != nil {
			message = strings.TrimSpace(string(rawError))
		}
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: message}}, IsError: true}, nil
	}
	if rawText, exists := envelope["plain_text_response"]; exists {
		var message string
		if err := json.Unmarshal(rawText, &message); err != nil {
			return nil, fmt.Errorf("llama.cpp returned an invalid text tool result: %w", err)
		}
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: message}}}, nil
	}
	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(content)}}}, nil
}
