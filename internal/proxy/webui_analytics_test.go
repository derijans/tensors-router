package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
)

func TestWebUIProxyRecordsInferenceAnalytics(t *testing.T) {
	service := newAnalyticsWebUIService(t)
	loadWebUIForTest(t, service, "kobold-lite", "text", "")
	service.webUI.session.set("kobold-lite", true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/webuis/kobold-lite/api/v1/generate", strings.NewReader(`{"prompt":"hi"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status %d body %s", recorder.Code, recorder.Body.String())
	}

	response := queryProxyAnalytics(t, service.analytics.store)
	if response.Summary.RequestCount != 1 {
		t.Fatalf("webui inference was not recorded %#v", response.Summary)
	}
	if response.Summary.InputTokens != 11 || response.Summary.OutputTokens != 153 {
		t.Fatalf("unexpected webui analytics summary %#v", response.Summary)
	}
	event := recentEventOfType(t, response, routeranalytics.EventTypeRequest)
	if event.ModelID != "text" || event.Section != routeranalytics.SectionLLM {
		t.Fatalf("unexpected webui recent analytics %#v", event)
	}
	if event.Route != "/api/*" {
		t.Fatalf("webui proxy prefix leaked into the route %q", event.Route)
	}
}

func TestWebUIProxyStreamRecordsUsageTheClientNeverSees(t *testing.T) {
	service := newAnalyticsWebUIService(t)
	loadWebUIForTest(t, service, "kobold-lite", "text", "")
	service.webUI.session.set("kobold-lite", true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/webuis/kobold-lite/v1/chat/completions", strings.NewReader(`{"messages":[],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status %d body %s", recorder.Code, recorder.Body.String())
	}

	if strings.Contains(recorder.Body.String(), "usage") {
		t.Fatalf("the injected usage chunk must not reach the webui %s", recorder.Body.String())
	}
	response := queryProxyAnalytics(t, service.analytics.store)
	if response.Summary.InputTokens != 2 || response.Summary.OutputTokens != 3 {
		t.Fatalf("webui stream usage was not recorded %#v", response.Summary)
	}
}

func TestWebUIProxySkipsStaticAssets(t *testing.T) {
	service := newAnalyticsWebUIService(t)
	loadWebUIForTest(t, service, "kobold-lite", "text", "")
	service.webUI.session.set("kobold-lite", true)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/webuis/kobold-lite/assets/app.js", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status %d body %s", recorder.Code, recorder.Body.String())
	}

	response := queryProxyAnalytics(t, service.analytics.store)
	if response.Summary.RequestCount != 0 {
		t.Fatalf("static asset must not be recorded %#v", response.Recent)
	}
	for _, event := range response.Recent {
		if event.EventType == routeranalytics.EventTypeRequest {
			t.Fatalf("static asset must not be recorded %#v", event)
		}
	}
}

func newAnalyticsWebUIService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "text.kcpps"), []byte(`{"model_param":"text.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"ready"}]}`))
		case r.URL.Path == "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
			if strings.Contains(string(body), `"include_usage":true`) {
				_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case r.URL.Path == "/api/v1/generate":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"text":"hi","prompt_tokens":11,"completion_tokens":153}]}`))
		default:
			_, _ = w.Write([]byte("local:" + r.URL.Path))
		}
	}))
	t.Cleanup(backendServer.Close)
	service := NewService(ServiceConfig{
		Backend: &fakeBackend{url: mustParseURL(t, backendServer.URL), healthy: true},
		Catalog: catalog.New(dir),
		NodeID:  "local",
		Logger:  log.New(io.Discard, "", 0),
	})
	service.analytics.store = newProxyAnalyticsStore(t, "local")
	return service
}
