package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

type backendClient struct {
	gateway *Gateway
	target  Target
}

func (client *backendClient) request(ctx context.Context, method string, endpoint string, payload any) ([]byte, error) {
	target := cloneURL(client.target.URL)
	target.Path = path.Join(target.Path, endpoint)
	target.RawQuery = ""
	target.Fragment = ""
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		if int64(len(encoded)) > client.gateway.maxRequestBodyBytes {
			return nil, fmt.Errorf("backend MCP request exceeds %d bytes", client.gateway.maxRequestBodyBytes)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.gateway.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.ContentLength > client.gateway.maxResponseBodyBytes {
		return nil, fmt.Errorf("backend MCP response exceeds %d bytes", client.gateway.maxResponseBodyBytes)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, client.gateway.maxResponseBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > client.gateway.maxResponseBodyBytes {
		return nil, fmt.Errorf("backend MCP response exceeds %d bytes", client.gateway.maxResponseBodyBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("backend MCP request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(content)))
	}
	return content, nil
}
