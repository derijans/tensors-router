package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

const routerPrivateUsageKey = "prompt_tokens_per_second"

var routerPrivateUsagePath = []string{"usage", routerPrivateUsageKey}

func clientStreamUsage(response *http.Response, injected bool) *http.Response {
	if response == nil || response.Body == nil || !isEventStream(response.Header) {
		return response
	}
	response.Body = &clientUsageFilter{source: response.Body, dropUsage: injected}
	return response
}

type clientUsageFilter struct {
	source        io.ReadCloser
	dropUsage     bool
	ready         bytes.Buffer
	line          []byte
	droppedUsage  bool
	sourceDrained bool
}

func (filter *clientUsageFilter) Read(p []byte) (int, error) {
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

func (filter *clientUsageFilter) Close() error {
	return filter.source.Close()
}

func (filter *clientUsageFilter) consume(chunk []byte) {
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

func (filter *clientUsageFilter) flushRemainder() {
	if len(filter.line) > 0 {
		filter.emitLine(filter.line)
		filter.line = nil
	}
}

func (filter *clientUsageFilter) emitLine(line []byte) {
	text := strings.TrimRight(string(line), "\r\n")
	if filter.droppedUsage && text == "" {
		filter.droppedUsage = false
		return
	}
	filter.droppedUsage = false
	switch {
	case filter.dropUsage && isUsageOnlyEventLine(text):
		filter.droppedUsage = true
	case !filter.dropUsage && strings.Contains(text, routerPrivateUsageKey) && isUsageOnlyEventLine(text):
		_, _ = filter.ready.Write(withoutRouterPrivateUsage(line, text))
	default:
		_, _ = filter.ready.Write(line)
	}
}

func withoutRouterPrivateUsage(line []byte, text string) []byte {
	payload, _ := strings.CutPrefix(text, "data:")
	public, removed := withoutJSONMember([]byte(strings.TrimSpace(payload)), routerPrivateUsagePath)
	if !removed {
		return line
	}
	rewritten := make([]byte, 0, len(line))
	rewritten = append(rewritten, serverSentEventDataPrefix...)
	rewritten = append(rewritten, public...)
	return append(rewritten, line[len(text):]...)
}
