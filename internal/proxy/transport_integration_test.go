package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/recipes"
	"tensors-router/internal/transportbody"
)

func TestStreamingRequestReachesBackendBeforeClientEOF(t *testing.T) {
	backendStarted := make(chan struct{}, 1)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := make([]byte, 10)
		if _, err := io.ReadFull(r.Body, prefix); err != nil {
			t.Error(err)
			return
		}
		backendStarted <- struct{}{}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[]}`))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)

	reader, writer := io.Pipe()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", reader)
	request.ContentLength = 100
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Tensors-Model", "llm")
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		service.ServeHTTP(recorder, request)
		close(done)
	}()

	if _, err := writer.Write(bytes.Repeat([]byte("a"), 10)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-backendStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not receive the stream before client EOF")
	}
	if _, err := writer.Write(bytes.Repeat([]byte("b"), 90)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streaming request did not finish")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"model":"llm"`) {
		t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMillionTokenJSONRecipeRewritesSelectorWithoutBufferingPayload(t *testing.T) {
	var received atomic.Int64
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		received.Store(int64(len(content)))
		if !bytes.Contains(content, []byte(`"model":"llm"`)) || bytes.Contains(content, []byte(`"model":"public"`)) {
			t.Errorf("selector was not rewritten: %.100s", content)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llm","choices":[]}`))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)
	store, err := recipes.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(recipes.Recipe{
		ID:       "recipe",
		PublicID: "public",
		Text: &recipes.Component{
			Kind:           recipes.KindText,
			NodeID:         "local",
			ModelID:        "llm",
			ConfigFilename: "llm.kcpps",
		},
	}, false); err != nil {
		t.Fatal(err)
	}
	service.recipeStore = store
	prompt := strings.Repeat("x ", 1_000_000)
	body := `{"model":"conflicting-selector","prompt":"` + prompt + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tensors-Model", "public")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || received.Load() < int64(len(prompt)) {
		t.Fatalf("large JSON was not streamed status=%d received=%d body=%s", recorder.Code, received.Load(), recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"public"`) {
		t.Fatalf("public response selector was not restored: %s", recorder.Body.String())
	}
}

func TestStreamingLimitsReturnSpecifiedErrors(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "64")
		_, _ = w.Write(bytes.Repeat([]byte("x"), 64))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)

	selectorRequired := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	service.ServeHTTP(selectorRequired, request)
	if selectorRequired.Code != http.StatusBadRequest || !strings.Contains(selectorRequired.Body.String(), "streaming_model_selector_required") {
		t.Fatalf("unexpected selector response %d %s", selectorRequired.Code, selectorRequired.Body.String())
	}

	tooLarge := httptest.NewRecorder()
	service.transportLimits.MaxRequestBytes = 64
	request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 65)))
	request.Header.Set("X-Tensors-Model", "llm")
	service.ServeHTTP(tooLarge, request)
	if tooLarge.Code != http.StatusRequestEntityTooLarge || !strings.Contains(tooLarge.Body.String(), "request_too_large") {
		t.Fatalf("unexpected request cap response %d %s", tooLarge.Code, tooLarge.Body.String())
	}

	responseTooLarge := httptest.NewRecorder()
	service.transportLimits.MaxResponseBytes = 32
	request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Tensors-Model", "llm")
	service.ServeHTTP(responseTooLarge, request)
	if responseTooLarge.Code != http.StatusBadGateway || !strings.Contains(responseTooLarge.Body.String(), "response_too_large") {
		t.Fatalf("unexpected response cap response %d %s", responseTooLarge.Code, responseTooLarge.Body.String())
	}
}

func TestTransportWorkingSetsUseSharedBudget(t *testing.T) {
	service := &Service{
		transportLimits: transportbody.Limits{
			ReplayBufferBytes: 8,
			MemoryBudgetBytes: transportbody.TransformationWorkingSet + 1,
			MaxRequestBytes:   64,
			SelectorScanBytes: 8,
		},
	}
	service.transportBudget = transportbody.NewBudget(service.transportLimits.MemoryBudgetBytes)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 20)))
	request.Header.Set("X-Tensors-Model", "llm")

	first, ok := service.reserveTransportWorkingSet(request)
	if !ok {
		t.Fatal("first working set was rejected")
	}
	if _, ok := service.reserveTransportWorkingSet(request); ok {
		first.Release()
		t.Fatal("concurrent working set exceeded the budget")
	}
	first.Release()
	second, ok := service.reserveTransportWorkingSet(request)
	if !ok {
		t.Fatal("released capacity was not reusable")
	}
	second.Release()
}

func TestStreamingRetriesOnlyBeforeAttemptConsumesBody(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[]}`))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)
	base := http.DefaultTransport
	var attempts atomic.Int32
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost && attempts.Add(1) == 1 {
			return nil, errors.New("connect failed before body read")
		}
		return base.RoundTrip(request)
	})}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Tensors-Model", "llm")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || attempts.Load() != 2 {
		t.Fatalf("pre-consumption retry failed status=%d attempts=%d body=%s", recorder.Code, attempts.Load(), recorder.Body.String())
	}

	service, _ = newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	useTinyTransportLimits(service)
	attempts.Store(0)
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet {
			return base.RoundTrip(request)
		}
		attempts.Add(1)
		buffer := make([]byte, 1)
		_, _ = request.Body.Read(buffer)
		return nil, errors.New("connection lost")
	})}
	request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 20)))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Tensors-Model", "llm")
	recorder = httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadGateway || attempts.Load() != 1 || !strings.Contains(recorder.Body.String(), "non_replayable_transport_error") {
		t.Fatalf("consumed stream retried status=%d attempts=%d body=%s", recorder.Code, attempts.Load(), recorder.Body.String())
	}
}

func TestBackendHeaderAllowlistStripsCredentialsAndForwardingMetadata(t *testing.T) {
	seen := make(chan http.Header, 1)
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"text":"ok"}]}`))
	}), map[string]string{"llm": `{"model_param":"llm.gguf"}`})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer caller")
	request.Header.Set("Cookie", "admin=secret")
	request.Header.Set("Forwarded", "for=203.0.113.1")
	request.Header.Set("X-Forwarded-For", "203.0.113.1")
	request.Header.Set("X-Real-IP", "203.0.113.1")
	request.Header.Set("Proxy-Authorization", "Basic secret")
	request.Header.Set("Connection", "X-Leak")
	request.Header.Set("X-Leak", "secret")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
	header := <-seen
	for _, key := range []string{"Authorization", "Cookie", "Forwarded", "X-Forwarded-For", "X-Real-IP", "Proxy-Authorization", "X-Leak"} {
		if header.Get(key) != "" {
			t.Fatalf("backend received forbidden header %s=%q", key, header.Get(key))
		}
	}
	if header.Get("Content-Type") != "application/json" {
		t.Fatal("content type was not forwarded")
	}
}

func useTinyTransportLimits(service *Service) {
	service.transportLimits = transportbody.Limits{
		ReplayBufferBytes: 8,
		MemoryBudgetBytes: transportbody.TransformationWorkingSet + 64,
		MaxRequestBytes:   4 * transportbody.MiB,
		MaxResponseBytes:  transportbody.MiB,
		SelectorScanBytes: 8,
	}
	service.transportBudget = transportbody.NewBudget(service.transportLimits.MemoryBudgetBytes)
}
