package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"tensors-router/internal/openai"
)

func modelFromRequest(body []byte, r *http.Request) (string, bool, error) {
	if len(body) > 0 && requestBodyLooksJSON(body, r) {
		modelID, ok, err := openai.ModelFromJSON(body)
		if err != nil || ok {
			return modelID, ok, err
		}
	}
	if modelID := strings.TrimSpace(r.URL.Query().Get("model")); modelID != "" {
		return modelID, true, nil
	}
	if modelID := strings.TrimSpace(r.Header.Get("X-Tensors-Model")); modelID != "" {
		return modelID, true, nil
	}
	return "", false, nil
}

func audioModelFromRequest(body []byte, r *http.Request) (string, bool, error) {
	if len(body) > 0 && requestBodyLooksJSON(body, r) {
		modelID, ok, err := openai.ModelFromJSON(body)
		if err != nil || ok {
			return modelID, ok, err
		}
	}
	if modelID, ok, err := multipartModelFromRequest(body, r); err != nil || ok {
		return modelID, ok, err
	}
	if modelID := strings.TrimSpace(r.URL.Query().Get("model")); modelID != "" {
		return modelID, true, nil
	}
	if modelID := strings.TrimSpace(r.Header.Get("X-Tensors-Model")); modelID != "" {
		return modelID, true, nil
	}
	return "", false, nil
}

func multipartModelFromRequest(body []byte, r *http.Request) (string, bool, error) {
	contentType := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		return "", false, nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", false, fmt.Errorf("multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if part.FormName() != "model" {
			_ = part.Close()
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, 1024))
		_ = part.Close()
		if err != nil {
			return "", false, err
		}
		modelID := strings.TrimSpace(string(value))
		return modelID, modelID != "", nil
	}
}

func imageModelFromRequest(body []byte, r *http.Request) (string, bool, error) {
	if len(body) > 0 && requestBodyLooksJSON(body, r) {
		modelID, ok, err := imageModelFromJSON(body)
		if err != nil || ok {
			return modelID, ok, err
		}
	}
	if modelID := strings.TrimSpace(r.URL.Query().Get("model")); modelID != "" {
		return modelID, true, nil
	}
	if modelID := strings.TrimSpace(r.URL.Query().Get("sd_model_checkpoint")); modelID != "" {
		return modelID, true, nil
	}
	if modelID := strings.TrimSpace(r.Header.Get("X-Tensors-Model")); modelID != "" {
		return modelID, true, nil
	}
	return "", false, nil
}

func requestBodyLooksJSON(body []byte, r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func imageModelFromJSON(body []byte) (string, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", false, err
	}
	if modelID, ok, err := jsonStringSelector(raw, "model"); err != nil || ok {
		return modelID, ok, err
	}
	if modelID, ok, err := jsonStringSelector(raw, "sd_model_checkpoint"); err != nil || ok {
		return modelID, ok, err
	}
	overrideSettings, ok := raw["override_settings"]
	if !ok {
		return "", false, nil
	}
	var overrideRaw map[string]json.RawMessage
	if err := json.Unmarshal(overrideSettings, &overrideRaw); err != nil {
		return "", false, fmt.Errorf("override_settings must be an object")
	}
	return jsonStringSelector(overrideRaw, "sd_model_checkpoint")
}

func jsonStringSelector(raw map[string]json.RawMessage, key string) (string, bool, error) {
	value, ok := raw[key]
	if !ok {
		return "", false, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", true, fmt.Errorf("%s must be a string", key)
	}
	text = strings.TrimSpace(text)
	return text, text != "", nil
}
