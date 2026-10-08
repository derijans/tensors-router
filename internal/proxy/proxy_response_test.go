package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamingResponseRewritesModel(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"model\":\"koboldcpp/backend\",\"choices\":[]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected content type protection %q", recorder.Header().Get("X-Content-Type-Options"))
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("stream model was not rewritten: %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "koboldcpp/backend") {
		t.Fatalf("unexpected body %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "data: [DONE]") {
		t.Fatalf("done event missing: %q", recorder.Body.String())
	}
}

func TestStreamingResponseEscapesRewrittenModel(t *testing.T) {
	recorder := httptest.NewRecorder()
	response := testHTTPResponse(http.StatusOK, "text/event-stream", "data: {\"model\":\"backend\",\"choices\":[{\"delta\":{\"content\":\"<script>alert(1)</script>\"}}]}\n\n")
	if err := writeProxyResponse(recorder, response, `bad<script>`, true); err != nil {
		t.Fatal(err)
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("stream reflected raw model id: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `bad\u003cscript\u003e`) {
		t.Fatalf("stream did not escape model id: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `\u003cscript\u003ealert(1)\u003c/script\u003e`) {
		t.Fatalf("stream did not escape reflected content: %s", recorder.Body.String())
	}
}

func TestStreamingResponseDropsInvalidDataLine(t *testing.T) {
	recorder := httptest.NewRecorder()
	response := testHTTPResponse(http.StatusOK, "text/event-stream", "data: <script>alert(1)</script>\n\n")
	if err := writeProxyResponse(recorder, response, "a", true); err != nil {
		t.Fatal(err)
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("invalid event data was reflected: %s", recorder.Body.String())
	}
}

func TestProxyResponsePreservesBinaryBodyAndAddsContentTypeProtection(t *testing.T) {
	body := []byte{0, 1, 2, 3, 255}
	response := testHTTPResponse(http.StatusOK, "application/octet-stream", string(body))
	recorder := httptest.NewRecorder()

	if err := writeProxyResponse(recorder, response, "", false); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if !bytes.Equal(recorder.Body.Bytes(), body) {
		t.Fatalf("binary response changed from %v to %v", body, recorder.Body.Bytes())
	}
	if recorder.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected content type protection %q", recorder.Header().Get("X-Content-Type-Options"))
	}
}

func TestModelProxyResponseHandlesMissingBackendResponse(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
	}{
		{name: "nil response"},
		{
			name: "nil body",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			if err := writeModelProxyResponse(recorder, testCase.response, "a", true); err != nil {
				t.Fatal(err)
			}

			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "backend returned no response") {
				t.Fatalf("missing backend error body: %s", recorder.Body.String())
			}
		})
	}
}

func TestModelRequestRejectsUnsupportedBackendContentType(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<script>alert(1)</script>`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[{"role":"user","content":"<script>alert(1)</script>"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("unsupported backend response reflected body: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "invalid json") {
		t.Fatalf("missing safe backend error: %s", recorder.Body.String())
	}
}

func TestModelRequestEscapesValidJSONBackendContent(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"<script>alert(1)</script>"}}]}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[{"role":"user","content":"<script>alert(1)</script>"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("json response reflected raw script: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `\u003cscript\u003ealert(1)\u003c/script\u003e`) {
		t.Fatalf("json response did not escape reflected content: %s", recorder.Body.String())
	}
}
