package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnlinkedTextRequestIsNeverQueued(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
		}),
		http.NotFoundHandler(),
		map[string]string{
			"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"text-only","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if service.textQueue != nil {
		if stats := service.textQueue.Stats(); len(stats) != 0 {
			t.Fatalf("stats = %+v, want nothing queued for an unlinked model with no registry at all", stats)
		}
	}
}
