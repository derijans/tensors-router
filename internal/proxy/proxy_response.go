package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"strings"

	"tensors-router/internal/openai"
	"tensors-router/internal/transportbody"
)

var errMissingBackendResponse = errors.New("backend returned no response")

func writeModelProxyResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error {
	return writeModelProxyResponseWithLimit(w, response, virtualModelID, rewriteModel, transportbody.DefaultLimits().MaxResponseBytes)
}

func (service *Service) writeModelProxyResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error {
	return writeModelProxyResponseWithLimit(w, response, virtualModelID, rewriteModel, service.transportLimits.MaxResponseBytes)
}

func writeModelProxyResponseWithLimit(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool, maxResponseBytes int64) error {
	if !rewriteModel {
		return writeProxyResponseWithLimit(w, response, virtualModelID, false, maxResponseBytes)
	}
	if response == nil || response.Body == nil {
		return writeMissingBackendResponse(w)
	}
	defer response.Body.Close()
	if response.ContentLength > maxResponseBytes {
		writeTransportError(w, transportbody.ErrResponseTooLarge)
		return nil
	}
	response.Body = limitProxyResponseBody(response.Body, maxResponseBytes)
	if isEventStream(response.Header) {
		return writeEventStreamResponse(w, response, virtualModelID)
	}
	if isNDJSONResponse(response.Header) {
		return writeNDJSONResponseWithVirtualModel(w, response, virtualModelID)
	}
	return writeJSONResponseWithVirtualModel(w, response, virtualModelID, maxResponseBytes)
}

func responseWithRelease(response *http.Response, release func()) *http.Response {
	if response == nil {
		release()
		return response
	}
	if response.Body == nil {
		release()
		return response
	}
	response.Body = &releaseReadCloser{
		ReadCloser: response.Body,
		release:    release,
	}
	return response
}

func writeProxyResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error {
	return writeProxyResponseWithLimit(w, response, virtualModelID, rewriteModel, transportbody.DefaultLimits().MaxResponseBytes)
}

func (service *Service) writeProxyResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error {
	return writeProxyResponseWithLimit(w, response, virtualModelID, rewriteModel, service.transportLimits.MaxResponseBytes)
}

func writeProxyResponseWithLimit(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool, maxResponseBytes int64) error {
	if response == nil || response.Body == nil {
		return writeMissingBackendResponse(w)
	}
	defer response.Body.Close()
	if response.ContentLength > maxResponseBytes {
		writeTransportError(w, transportbody.ErrResponseTooLarge)
		return nil
	}
	response.Body = limitProxyResponseBody(response.Body, maxResponseBytes)

	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isEventStream(response.Header) {
		return writeEventStreamResponse(w, response, virtualModelID)
	}
	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isNDJSONResponse(response.Header) {
		return writeNDJSONResponseWithVirtualModel(w, response, virtualModelID)
	}
	if rewriteModel && response.StatusCode >= 200 && response.StatusCode < 300 && isJSONResponse(response.Header) {
		return writeJSONResponseWithVirtualModel(w, response, virtualModelID, maxResponseBytes)
	}

	copyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)

	_, err := transportbody.CopyFlushing(w, response.Body)
	return err
}

func limitProxyResponseBody(body io.ReadCloser, maxResponseBytes int64) io.ReadCloser {
	return &readerReadCloser{
		Reader: transportbody.LimitResponse(body, maxResponseBytes),
		Closer: body,
	}
}

func writeMissingBackendResponse(w http.ResponseWriter) error {
	openai.WriteError(w, http.StatusBadGateway, "backend_error", errMissingBackendResponse.Error())
	return nil
}

func writeJSONResponseWithVirtualModel(w http.ResponseWriter, response *http.Response, virtualModelID string, maxResponseBytes int64) error {
	if response.ContentLength < 0 || response.ContentLength > backendResponseMetadataLimit {
		return streamJSONResponseWithVirtualModel(w, response, virtualModelID, maxResponseBytes)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if !json.Valid(body) {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", "backend returned invalid json")
		return nil
	}

	body = rewriteJSONModel(body, virtualModelID)
	body = htmlEscapeJSON(body)
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	_, err = w.Write(body)
	return err
}

func streamJSONResponseWithVirtualModel(w http.ResponseWriter, response *http.Response, virtualModelID string, maxResponseBytes int64) error {
	source := transportbody.NewJSONTransformReadCloser(response.Body, transportbody.JSONRewrite{
		Replacements: map[string]transportbody.StringReplacement{
			transportbody.PathModel: {To: virtualModelID},
		},
		EscapeHTML: true,
	})
	defer source.Close()
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	_, err := transportbody.CopyResponseFlushing(w, source, maxResponseBytes)
	return err
}

func writeEventStreamResponse(w http.ResponseWriter, response *http.Response, virtualModelID string) error {
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)

	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := writeEventStreamLine(w, scanner.Text(), virtualModelID); err != nil {
			return err
		}
		if scanner.Text() == "" && flusher != nil {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func rewriteJSONModel(body []byte, virtualModelID string) []byte {
	rewritten, err := transportbody.RewriteJSON(body, transportbody.JSONRewrite{
		Replacements: map[string]transportbody.StringReplacement{
			transportbody.PathModel: {To: virtualModelID},
		},
	})
	if err != nil {
		return body
	}
	return rewritten
}

func writeEventStreamLine(w io.Writer, line string, virtualModelID string) error {
	switch {
	case line == "":
		_, err := io.WriteString(w, "\n")
		return err
	case strings.HasPrefix(line, "data: "):
		return writeEventDataLine(w, strings.TrimPrefix(line, "data: "), virtualModelID)
	case strings.HasPrefix(line, "data:"):
		return writeEventDataLine(w, strings.TrimPrefix(line, "data:"), virtualModelID)
	case json.Valid([]byte(line)):
		return writeBareJSONStreamLine(w, line, virtualModelID)
	default:
		_, err := io.WriteString(w, html.EscapeString(line)+"\n")
		return err
	}
}

func writeBareJSONStreamLine(w io.Writer, line string, virtualModelID string) error {
	rewritten := htmlEscapeJSON(rewriteJSONModel([]byte(line), virtualModelID))
	_, err := w.Write(append(rewritten, '\n'))
	return err
}

func writeEventDataLine(w io.Writer, data string, virtualModelID string) error {
	rewritten, ok := rewriteEventDataModel(data, virtualModelID)
	if !ok {
		return nil
	}
	_, err := io.WriteString(w, "data: "+string(rewritten)+"\n")
	return err
}

func rewriteEventDataModel(data string, virtualModelID string) ([]byte, bool) {
	if strings.TrimSpace(data) == "[DONE]" {
		return []byte("[DONE]"), true
	}
	body := []byte(data)
	if !json.Valid(body) {
		return nil, false
	}
	return htmlEscapeJSON(rewriteJSONModel(body, virtualModelID)), true
}

func htmlEscapeJSON(body []byte) []byte {
	if !bytes.ContainsAny(body, "<>&") {
		return body
	}
	var buffer bytes.Buffer
	buffer.Grow(len(body))
	json.HTMLEscape(&buffer, body)
	return buffer.Bytes()
}

func copyResponseHeaders(dst http.Header, src http.Header) {
	blocked := connectionHeaderNames(src)
	for key, values := range src {
		if _, skip := blocked[strings.ToLower(key)]; skip || strings.EqualFold(key, "Set-Cookie") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	dst.Set("X-Content-Type-Options", "nosniff")
}

func isJSONResponse(header http.Header) bool {
	return strings.Contains(strings.ToLower(header.Get("Content-Type")), "application/json")
}

func isEventStream(header http.Header) bool {
	return strings.Contains(strings.ToLower(header.Get("Content-Type")), "text/event-stream")
}

type readerReadCloser struct {
	io.Reader
	Closer io.Closer
}

func (reader *readerReadCloser) Close() error {
	return reader.Closer.Close()
}
