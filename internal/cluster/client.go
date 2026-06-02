package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	token  string
	client *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (client *Client) FetchSnapshot(ctx context.Context, nodeURL string) (Snapshot, error) {
	var snapshot Snapshot
	err := client.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/models", nil, &snapshot)
	return snapshot, err
}

func (client *Client) Register(ctx context.Context, masterURL string, snapshot Snapshot) error {
	return client.JSON(ctx, http.MethodPost, masterURL, "/router/v1/node/register", snapshot, nil)
}

func (client *Client) Load(ctx context.Context, nodeURL string, modelID string) error {
	return client.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/load", map[string]string{"model": modelID}, nil)
}

func (client *Client) Unload(ctx context.Context, nodeURL string, modelID string) error {
	body := map[string]string{}
	if strings.TrimSpace(modelID) != "" {
		body["model"] = modelID
	}
	return client.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/unload", body, nil)
}

func (client *Client) JSON(ctx context.Context, method string, baseURL string, path string, requestBody any, responseBody any) error {
	target, err := joinedURL(baseURL, path)
	if err != nil {
		return err
	}

	var body io.Reader
	if requestBody != nil {
		content, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(content)
	}

	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}

	response, err := client.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("cluster request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(content)))
	}
	if responseBody == nil {
		return nil
	}
	return json.Unmarshal(content, responseBody)
}

func joinedURL(baseURL string, path string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/")
	parsed.RawQuery = ""
	return parsed.String(), nil
}
