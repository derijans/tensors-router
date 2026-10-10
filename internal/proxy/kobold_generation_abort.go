package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
)

const (
	koboldAbortPath    = "/api/extra/abort"
	koboldGenkeyField  = "genkey"
	koboldGenkeyBytes  = 12
	koboldAbortBodyMax = 1 << 10
)

var koboldAbortableGenerationPaths = map[string]struct{}{
	"/v1/chat/completions": {},
	"/v1/completions":      {},
	"/api/v1/generate":     {},
	koboldNativeStreamPath: {},
}

type koboldAbandonGuard struct {
	client     *http.Client
	backendURL *url.URL
	genkey     string
	disarm     func() bool
	once       sync.Once
}

func markKoboldGeneration(path string, backendMode string, body []byte) ([]byte, string) {
	if backendMode != BackendModeKobold {
		return body, ""
	}
	if _, abortable := koboldAbortableGenerationPaths[path]; !abortable {
		return body, ""
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return body, ""
	}
	if existing, ok := fields[koboldGenkeyField]; ok {
		var key string
		if json.Unmarshal(existing, &key) == nil && key != "" {
			return body, key
		}
	}
	key, err := newKoboldGenkey()
	if err != nil {
		return body, ""
	}
	encodedKey, err := json.Marshal(key)
	if err != nil {
		return body, ""
	}
	fields[koboldGenkeyField] = encodedKey
	marked, err := json.Marshal(fields)
	if err != nil {
		return body, ""
	}
	return marked, key
}

func newKoboldGenkey() (string, error) {
	raw := make([]byte, koboldGenkeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "tr-" + hex.EncodeToString(raw), nil
}

func guardAbandonedKoboldGeneration(ctx context.Context, client *http.Client, backendURL *url.URL, genkey string) *koboldAbandonGuard {
	if genkey == "" {
		return nil
	}
	guard := &koboldAbandonGuard{client: client, backendURL: backendURL, genkey: genkey}
	guard.disarm = context.AfterFunc(ctx, guard.abort)
	return guard
}

func (guard *koboldAbandonGuard) disarmOnceDelivered(response *http.Response, err error) (*http.Response, error) {
	if guard == nil {
		return response, err
	}
	if err != nil || response == nil || response.Body == nil {
		guard.disarm()
		return response, err
	}
	response.Body = &koboldDeliveredBody{ReadCloser: response.Body, delivered: guard.disarm}
	return response, nil
}

func (guard *koboldAbandonGuard) abort() {
	guard.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), koboldPerfRequestDeadline)
		defer cancel()
		target := *guard.backendURL
		target.Path = joinPath(target.Path, koboldAbortPath)
		target.RawQuery = ""
		payload, err := json.Marshal(map[string]string{koboldGenkeyField: guard.genkey})
		if err != nil {
			return
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
		if err != nil {
			return
		}
		request.Header.Set(headerContentType, mediaTypeJSON)
		response, err := guard.client.Do(request)
		if err != nil {
			return
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, koboldAbortBodyMax))
	})
}

type koboldDeliveredBody struct {
	io.ReadCloser
	delivered func() bool
}

func (body *koboldDeliveredBody) Read(p []byte) (int, error) {
	read, err := body.ReadCloser.Read(p)
	if err == io.EOF {
		body.delivered()
	}
	return read, err
}

func (body *koboldDeliveredBody) Close() error {
	body.delivered()
	return body.ReadCloser.Close()
}
