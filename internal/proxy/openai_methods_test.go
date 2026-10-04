package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIPostOnlyPathsRejectOtherMethods(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("backend received method mismatch")
	}), map[string]string{})
	for _, path := range []string{
		"/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/responses",
		"/v1/messages", "/v1/messages/count_tokens", "/v1/rerank", "/v1/reranking",
		"/v1/images/generations", "/v1/images/edits",
		"/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations",
	} {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("%s: status=%d allow=%q body=%s", path, recorder.Code, recorder.Header().Get("Allow"), recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"type":"invalid_request_error"`) {
			t.Fatalf("%s: unexpected error shape %s", path, recorder.Body.String())
		}
	}
}
