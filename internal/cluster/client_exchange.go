package cluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

type RawResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (client *Client) Exchange(ctx context.Context, method string, baseURL string, path string, requestJSON []byte) (RawResponse, error) {
	return client.exchange(ctx, client.client, method, baseURL, path, requestJSON)
}

func (client *Client) exchange(ctx context.Context, httpClient *http.Client, method string, baseURL string, path string, requestJSON []byte) (RawResponse, error) {
	target, err := client.joinedAllowedURL(baseURL, path)
	if err != nil {
		return RawResponse{}, err
	}
	if int64(len(requestJSON)) > maxClusterJSONBytes {
		return RawResponse{}, fmt.Errorf("cluster request body exceeds %d bytes", maxClusterJSONBytes)
	}
	var body io.Reader
	if requestJSON != nil {
		body = bytes.NewReader(requestJSON)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return RawResponse{}, err
	}
	if requestJSON != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client.setAuthorization(request)

	response, err := httpClient.Do(request)
	if err != nil {
		return RawResponse{}, err
	}
	defer response.Body.Close()
	content, err := readBoundedResponse(response)
	if err != nil {
		return RawResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return RawResponse{}, decodeRemoteError(response.StatusCode, content)
	}
	return RawResponse{StatusCode: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Body: content}, nil
}

func readBoundedResponse(response *http.Response) ([]byte, error) {
	if response.ContentLength > maxClusterJSONBytes {
		return nil, fmt.Errorf("cluster response body exceeds %d bytes", maxClusterJSONBytes)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxClusterJSONBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxClusterJSONBytes {
		return nil, fmt.Errorf("cluster response body exceeds %d bytes", maxClusterJSONBytes)
	}
	return content, nil
}
