package ollama

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

var methods = map[string]string{
	"/api/show":     http.MethodPost,
	"/api/generate": http.MethodPost,
	"/api/chat":     http.MethodPost,
	"/api/embed":    http.MethodPost,
	"/api/tags":     http.MethodGet,
	"/api/ps":       http.MethodGet,
	"/api/version":  http.MethodGet,
}

const maxBufferedErrorBytes = 64 * 1024

func Method(path string) (string, bool) {
	method, ok := methods[path]
	return method, ok
}

func WriteError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

type ErrorResponseWriter struct {
	destination http.ResponseWriter
	header      http.Header
	status      int
	body        bytes.Buffer
	overflowed  bool
	committed   bool
}

func NewErrorResponseWriter(destination http.ResponseWriter) *ErrorResponseWriter {
	return &ErrorResponseWriter{destination: destination, header: make(http.Header)}
}

func (writer *ErrorResponseWriter) Header() http.Header {
	return writer.header
}

func (writer *ErrorResponseWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	writer.status = status
	if status >= 200 && status < 400 {
		writer.commitHeaders()
	}
}

func (writer *ErrorResponseWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.committed {
		contentType := "application/json"
		if isNDJSON(writer.header.Get("Content-Type")) {
			contentType = "application/x-ndjson"
		}
		writer.destination.Header().Set("Content-Type", contentType)
		writer.destination.Header().Set("X-Content-Type-Options", "nosniff")
		return writer.destination.Write(body)
	}
	if writer.overflowed || len(body) > maxBufferedErrorBytes-writer.body.Len() {
		writer.overflowed = true
		writer.body.Reset()
		return len(body), nil
	}
	return writer.body.Write(body)
}

func (writer *ErrorResponseWriter) Flush() {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if flusher, ok := writer.destination.(http.Flusher); ok && writer.committed {
		flusher.Flush()
	}
}

func (writer *ErrorResponseWriter) Finish() {
	if writer.committed {
		return
	}
	status := writer.status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	message := http.StatusText(status)
	if message == "" {
		message = "request failed"
	}
	if !writer.overflowed {
		message = errorMessage(writer.body.Bytes(), message)
	}
	copyHeaders(writer.destination.Header(), writer.header)
	writer.destination.Header().Del("Content-Encoding")
	writer.destination.Header().Del("Content-Length")
	writer.destination.Header().Del("Content-Range")
	writer.destination.Header().Del("ETag")
	writer.committed = true
	WriteError(writer.destination, status, message)
}

func (writer *ErrorResponseWriter) commitHeaders() {
	copyHeaders(writer.destination.Header(), writer.header)
	contentType := "application/json"
	if isNDJSON(writer.header.Get("Content-Type")) {
		contentType = "application/x-ndjson"
	}
	writer.destination.Header().Set("Content-Type", contentType)
	writer.destination.Header().Set("X-Content-Type-Options", "nosniff")
	writer.destination.WriteHeader(writer.status)
	writer.committed = true
}

func isNDJSON(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "application/x-ndjson") || strings.Contains(contentType, "application/ndjson")
}

func errorMessage(body []byte, fallback string) string {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 {
		var message string
		if json.Unmarshal(envelope.Error, &message) == nil && strings.TrimSpace(message) != "" {
			return message
		}
		var detail struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Error, &detail) == nil && strings.TrimSpace(detail.Message) != "" {
			return detail.Message
		}
	}
	if message := strings.TrimSpace(string(body)); message != "" && !json.Valid(body) {
		return message
	}
	return fallback
}

func copyHeaders(destination http.Header, source http.Header) {
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}
