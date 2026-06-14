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
	"sync"
	"time"
)

type Client struct {
	token   string
	client  *http.Client
	mu      sync.RWMutex
	allowed map[string]string
}

func NewClient(token string, baseURLs ...string) *Client {
	client := &Client{
		token: token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		allowed: map[string]string{},
	}
	_ = client.AllowBaseURLs(baseURLs...)
	return client
}

func (client *Client) AllowBaseURLs(baseURLs ...string) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	for _, baseURL := range baseURLs {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			continue
		}
		normalized, err := NormalizeBaseURL(baseURL)
		if err != nil {
			return err
		}
		client.allowed[normalized] = normalized
	}
	return nil
}

func (client *Client) AuthorizedBaseURL(baseURL string) (string, error) {
	return client.allowedBaseURL(baseURL)
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
	target, err := client.joinedAllowedURL(baseURL, path)
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

func (client *Client) joinedAllowedURL(baseURL string, path string) (string, error) {
	allowedBaseURL, err := client.allowedBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	return joinedURL(allowedBaseURL, path)
}

func (client *Client) allowedBaseURL(baseURL string) (string, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	client.mu.RLock()
	allowed, ok := client.allowed[normalized]
	client.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("cluster target %q is not allowed", normalized)
	}
	return allowed, nil
}

func joinedURL(baseURL string, path string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	requestPath, requestQuery, _ := strings.Cut(path, "?")
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(requestPath, "/")
	parsed.RawQuery = requestQuery
	return parsed.String(), nil
}

func NormalizeBaseURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("cluster target scheme must be http or https")
	}
	if parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("cluster target host is invalid")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func BaseURLEqual(left string, right string) bool {
	normalizedLeft, leftErr := NormalizeBaseURL(left)
	normalizedRight, rightErr := NormalizeBaseURL(right)
	return leftErr == nil && rightErr == nil && normalizedLeft == normalizedRight
}
