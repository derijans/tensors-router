package mcp

import "net/http"

type boundedResponseWriter struct {
	header     http.Header
	statusCode int
	body       []byte
	limit      int64
	overflowed bool
}

func newBoundedResponseWriter(limit int64) *boundedResponseWriter {
	return &boundedResponseWriter{header: make(http.Header), limit: limit}
}

func (writer *boundedResponseWriter) Header() http.Header {
	return writer.header
}

func (writer *boundedResponseWriter) WriteHeader(statusCode int) {
	if writer.statusCode == 0 {
		writer.statusCode = statusCode
	}
}

func (writer *boundedResponseWriter) Write(content []byte) (int, error) {
	if writer.statusCode == 0 {
		writer.statusCode = http.StatusOK
	}
	remaining := writer.limit - int64(len(writer.body))
	if remaining > 0 {
		captured := int64(len(content))
		if captured > remaining {
			captured = remaining
		}
		writer.body = append(writer.body, content[:int(captured)]...)
	}
	if int64(len(content)) > remaining {
		writer.overflowed = true
	}
	return len(content), nil
}

func (writer *boundedResponseWriter) writeTo(target http.ResponseWriter) error {
	for key, values := range writer.header {
		target.Header()[key] = append([]string(nil), values...)
	}
	statusCode := writer.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	target.WriteHeader(statusCode)
	_, err := target.Write(writer.body)
	return err
}
