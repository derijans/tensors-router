package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
)

const streamUsageOption = `"stream_options":{"include_usage":true}`

type streamUsageProbe struct {
	Stream        *bool           `json:"stream"`
	StreamOptions json.RawMessage `json:"stream_options"`
}

func backendReportsStreamUsage(backendMode string) bool {
	return backendMode == BackendModeLlamaSDCPP
}

func requestStreamsOpenAIUsage(path string) bool {
	return path == "/v1/chat/completions" || path == "/v1/completions"
}

func injectStreamUsageOption(body []byte, path string, backendMode string) ([]byte, bool) {
	if routerReportsKoboldStreamUsage(path, backendMode) {
		return body, true
	}
	if !requestStreamsOpenAIUsage(path) || backendReportsStreamUsage(backendMode) {
		return body, false
	}
	trimmed := bytes.TrimRight(body, " \t\r\n")
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' || !json.Valid(trimmed) {
		return body, false
	}
	var probe streamUsageProbe
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return body, false
	}
	if probe.Stream == nil || !*probe.Stream || len(probe.StreamOptions) > 0 {
		return body, false
	}
	injected := make([]byte, 0, len(trimmed)+len(streamUsageOption)+1)
	injected = append(injected, trimmed[:len(trimmed)-1]...)
	if bytes.ContainsFunc(trimmed[1:len(trimmed)-1], func(r rune) bool { return !isJSONSpace(r) }) {
		injected = append(injected, ',')
	}
	injected = append(injected, streamUsageOption...)
	injected = append(injected, '}')
	return injected, true
}

func isJSONSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func isUsageOnlyEventLine(line string) bool {
	payload, found := strings.CutPrefix(line, "data:")
	if !found {
		return false
	}
	payload = strings.TrimSpace(payload)
	if payload == "" || payload == "[DONE]" {
		return false
	}
	var chunk struct {
		Choices []json.RawMessage `json:"choices"`
		Usage   json.RawMessage   `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return false
	}
	return len(chunk.Usage) > 0 && len(chunk.Choices) == 0
}
