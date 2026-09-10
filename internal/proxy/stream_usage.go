package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
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

func responseWithoutInjectedUsage(response *http.Response, injected bool) *http.Response {
	if !injected || response == nil || response.Body == nil || !isEventStream(response.Header) {
		return response
	}
	response.Body = &injectedUsageFilter{source: response.Body}
	return response
}

type injectedUsageFilter struct {
	source        io.ReadCloser
	ready         bytes.Buffer
	line          []byte
	droppedUsage  bool
	sourceDrained bool
}

func (filter *injectedUsageFilter) Read(p []byte) (int, error) {
	for filter.ready.Len() == 0 && !filter.sourceDrained {
		chunk := make([]byte, 32*1024)
		read, err := filter.source.Read(chunk[:])
		if read > 0 {
			filter.consume(chunk[:read])
		}
		if err == io.EOF {
			filter.sourceDrained = true
			filter.flushRemainder()
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if filter.ready.Len() == 0 && filter.sourceDrained {
		return 0, io.EOF
	}
	return filter.ready.Read(p)
}

func (filter *injectedUsageFilter) Close() error {
	return filter.source.Close()
}

func (filter *injectedUsageFilter) consume(chunk []byte) {
	filter.line = append(filter.line, chunk...)
	for {
		index := bytes.IndexByte(filter.line, '\n')
		if index < 0 {
			return
		}
		filter.emitLine(filter.line[:index+1])
		filter.line = filter.line[index+1:]
	}
}

func (filter *injectedUsageFilter) flushRemainder() {
	if len(filter.line) > 0 {
		filter.emitLine(filter.line)
		filter.line = nil
	}
}

func (filter *injectedUsageFilter) emitLine(line []byte) {
	text := strings.TrimRight(string(line), "\r\n")
	if filter.droppedUsage && text == "" {
		filter.droppedUsage = false
		return
	}
	filter.droppedUsage = false
	if isUsageOnlyEventLine(text) {
		filter.droppedUsage = true
		return
	}
	_, _ = filter.ready.Write(line)
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
