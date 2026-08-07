package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/transportbody"
)

func TestOllamaMethodsAndErrorShape(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("backend received method mismatch")
	}), map[string]string{})
	methods := map[string]string{
		"/api/show": http.MethodPost, "/api/generate": http.MethodPost, "/api/chat": http.MethodPost,
		"/api/embed": http.MethodPost, "/api/tags": http.MethodGet, "/api/ps": http.MethodGet, "/api/version": http.MethodGet,
	}
	for path, allowed := range methods {
		method := http.MethodGet
		if allowed == http.MethodGet {
			method = http.MethodPost
		}
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != allowed {
			t.Fatalf("%s: status=%d allow=%q", path, recorder.Code, recorder.Header().Get("Allow"))
		}
		if strings.TrimSpace(recorder.Body.String()) != `{"error":"method not allowed"}` {
			t.Fatalf("%s: unexpected Ollama error %s", path, recorder.Body.String())
		}
	}
}

func TestOllamaLocalErrorsDoNotUseOpenAIEnvelope(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("backend received invalid request")
	}), map[string]string{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(`{"prompt":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || strings.TrimSpace(recorder.Body.String()) != `{"error":"model is required"}` {
		t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestOllamaShowPreservesContractAndVerbose(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"verbose":true`) {
			t.Fatalf("verbose was not forwarded: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend-local","parameters":"temperature 0.8","details":{"family":"llama"},"model_info":{"general.architecture":"llama"},"capabilities":["completion"]}`))
	}), map[string]string{"text": `{"model_param":"C:\\models\\text.gguf"}`})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/show", strings.NewReader(`{"model":"text","verbose":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	body := recorder.Body.String()
	for _, field := range []string{`"model":"text"`, `"parameters":`, `"details":`, `"model_info":`, `"capabilities":`} {
		if !strings.Contains(body, field) {
			t.Fatalf("missing %s in %s", field, body)
		}
	}
}

func TestOllamaNDJSONRewritesEveryModelAndPassesErrorRecord(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/x-ndjson", "text/html"}},
		Body:       io.NopCloser(strings.NewReader("{\"model\":\"local-a\",\"response\":\"<script>alert(1)</script>\"}\n{\"model\":\"local-b\",\"done\":true}\n{\"error\":\"backend stopped\"}\n")),
	}
	recorder := httptest.NewRecorder()
	if err := writeModelProxyResponse(recorder, response, "public", true); err != nil {
		t.Fatal(err)
	}
	body := recorder.Body.String()
	if strings.Count(body, `"model":"public"`) != 2 || !strings.Contains(body, `{"error":"backend stopped"}`) {
		t.Fatalf("unexpected NDJSON %s", body)
	}
	if strings.Contains(body, "local-a") || strings.Contains(body, "local-b") {
		t.Fatalf("backend-local identifier leaked: %s", body)
	}
	if strings.Contains(body, "<script>") || !strings.Contains(body, `\u003cscript\u003e`) {
		t.Fatalf("backend HTML was not escaped: %s", body)
	}
	if got := recorder.Header().Values("Content-Type"); len(got) != 1 || got[0] != "application/x-ndjson" {
		t.Fatalf("unexpected content type %#v", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q", got)
	}
}

func TestOllamaTagsAndPSCollapseReplicasWithoutLocalIDLeakage(t *testing.T) {
	models := []cluster.Model{
		{PublicID: "public", LocalID: "local-a", HasLLM: true, Available: true, Loaded: true, ModelHash: strings.Repeat("a", 64), Created: 1, Size: 321},
		{PublicID: "public", LocalID: "local-b", HasLLM: true, Available: true, Loaded: true, ModelHash: strings.Repeat("a", 64), Created: 1},
		{PublicID: "down", LocalID: "secret-down", HasLLM: true, Available: false, Loaded: true, Created: 2},
	}
	tags := ollamaModels(models, false)
	if len(tags) != 2 || tags[0].Name != "down" || tags[1].Name != "public" {
		t.Fatalf("unexpected tags %#v", tags)
	}
	loaded := ollamaModels(models, true)
	if len(loaded) != 1 || loaded[0].Name != "public" || loaded[0].Model != "public" {
		t.Fatalf("unexpected running models %#v", loaded)
	}
	if !strings.HasPrefix(loaded[0].Digest, "sha256:") || loaded[0].ModifiedAt != time.Unix(1, 0).UTC() || loaded[0].Size != 321 {
		t.Fatalf("unexpected stable metadata %#v", loaded[0])
	}
}
func TestOllamaNDJSONPreservesBoundariesWhenModelLengthChanges(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/x-ndjson"}},
		Body:       io.NopCloser(strings.NewReader("{\"model\":\"x\",\"response\":\"one\"}\n{\"model\":\"x\",\"done\":true}")),
	}
	recorder := httptest.NewRecorder()
	if err := writeModelProxyResponse(recorder, response, "substantially-longer-public-model", true); err != nil {
		t.Fatal(err)
	}
	expected := "{\"model\":\"substantially-longer-public-model\",\"response\":\"one\"}\n{\"model\":\"substantially-longer-public-model\",\"done\":true}"
	if recorder.Body.String() != expected {
		t.Fatalf("record boundaries changed: got %q want %q", recorder.Body.String(), expected)
	}
}

func TestOllamaNDJSONRejectsOversizedRecord(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/x-ndjson"}},
		Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxNDJSONRecordBytes+1))),
	}
	recorder := httptest.NewRecorder()
	err := writeModelProxyResponse(recorder, response, "public", true)
	if err == nil || !strings.Contains(err.Error(), "record exceeds") {
		t.Fatalf("expected bounded record error, got %v", err)
	}
}

func TestOllamaTransportLimitsUseOllamaErrorContract(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("backend received rejected request")
	}), map[string]string{})

	t.Run("oversized request", func(t *testing.T) {
		service.transportLimits.MaxRequestBytes = 64
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(strings.Repeat("x", 65)))
		request.Header.Set("X-Tensors-Model", "text")
		service.ServeHTTP(recorder, request)
		assertOllamaError(t, recorder, http.StatusRequestEntityTooLarge, transportbody.ErrRequestTooLarge.Error())
	})

	t.Run("buffer exhaustion", func(t *testing.T) {
		service.transportLimits = transportbody.DefaultLimits()
		service.transportBudget = transportbody.NewBudget(transportbody.TransformationWorkingSet)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		assertOllamaError(t, recorder, http.StatusServiceUnavailable, transportbody.ErrBufferCapacity.Error())
	})
}

func TestOllamaOversizedBackendErrorUsesBoundedFallback(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"`+strings.Repeat("x", 70*1024)+`"}}`)
	}), map[string]string{"text": `{"model_param":"text.gguf"}`})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(`{"model":"text","prompt":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	assertOllamaError(t, recorder, http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
}

func assertOllamaError(t *testing.T, recorder *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected X-Content-Type-Options %q", recorder.Header().Get("X-Content-Type-Options"))
	}
	wantBody := `{"error":"` + message + `"}`
	if strings.TrimSpace(recorder.Body.String()) != wantBody {
		t.Fatalf("body=%s want=%s", recorder.Body.String(), wantBody)
	}
}
