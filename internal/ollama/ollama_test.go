package ollama

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorResponseWriterBoundsOversizedError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewErrorResponseWriter(recorder)
	writer.WriteHeader(http.StatusBadGateway)
	payload := []byte(`{"error":"` + strings.Repeat("x", maxBufferedErrorBytes) + `"}`)
	written, err := writer.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	if written != len(payload) {
		t.Fatalf("written=%d want=%d", written, len(payload))
	}
	writer.Finish()
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status=%d", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(recorder.Body.String()) != `{"error":"Bad Gateway"}` {
		t.Fatalf("unexpected body %s", recorder.Body.String())
	}
}

func TestErrorResponseWriterNormalizesSuccessfulContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        string
	}{
		{name: "backend html", contentType: "text/html", want: "application/json"},
		{name: "ndjson", contentType: "application/ndjson; charset=utf-8", want: "application/x-ndjson"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writer := NewErrorResponseWriter(recorder)
			writer.Header().Set("Content-Type", test.contentType)
			writer.WriteHeader(http.StatusOK)
			if _, err := writer.Write([]byte(`{"response":"ok"}`)); err != nil {
				t.Fatal(err)
			}
			writer.Finish()
			if got := recorder.Header().Get("Content-Type"); got != test.want {
				t.Fatalf("content type=%q want=%q", got, test.want)
			}
			if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options=%q", got)
			}
		})
	}
}
