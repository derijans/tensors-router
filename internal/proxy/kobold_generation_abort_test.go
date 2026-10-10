package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeKoboldGenerator struct {
	mu         sync.Mutex
	generation string
	aborted    chan string
	release    chan struct{}
}

func newFakeKoboldGenerator() *fakeKoboldGenerator {
	return &fakeKoboldGenerator{aborted: make(chan string, 4), release: make(chan struct{})}
}

func (generator *fakeKoboldGenerator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, _ := io.ReadAll(r.Body)
	switch r.URL.Path {
	case koboldAbortPath:
		var abort map[string]string
		_ = json.Unmarshal(payload, &abort)
		generator.aborted <- abort[koboldGenkeyField]
		_, _ = w.Write([]byte(`{"success":"true"}`))
	default:
		var generation map[string]any
		_ = json.Unmarshal(payload, &generation)
		generator.mu.Lock()
		generator.generation, _ = generation[koboldGenkeyField].(string)
		generator.mu.Unlock()
		select {
		case <-generator.release:
			w.Header().Set(headerContentType, mediaTypeJSON)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
		case <-r.Context().Done():
		}
	}
}

func (generator *fakeKoboldGenerator) generationKey() string {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	return generator.generation
}

func koboldForwardFixture(t *testing.T, generator *fakeKoboldGenerator) (*Service, *backendRuntime) {
	t.Helper()
	service, _ := newTestService(t, generator)
	runtime, err := service.runtimeForBackendMode(BackendModeKobold, readinessText)
	if err != nil {
		t.Fatal(err)
	}
	return service, runtime
}

func forwardChat(service *Service, runtime *backendRuntime, ctx context.Context, body string) (*http.Response, error) {
	original := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	original.Header.Set(headerContentType, mediaTypeJSON)
	return service.forward(runtime, ctx, original, []byte(body))
}

func TestAbandonedKoboldGenerationIsAbortedByItsOwnKey(t *testing.T) {
	generator := newFakeKoboldGenerator()
	service, runtime := koboldForwardFixture(t, generator)
	clientContext, leave := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = forwardChat(service, runtime, clientContext, `{"model":"m","messages":[]}`)
	}()
	waitUntil(t, func() bool { return generator.generationKey() != "" })

	leave()

	select {
	case aborted := <-generator.aborted:
		if aborted != generator.generationKey() {
			t.Fatalf("aborted key %q, but the generation ran under %q", aborted, generator.generationKey())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a generation whose client left was never aborted; KoboldCpp keeps generating and blocks the slot")
	}
	<-done
}

func TestDeliveredKoboldGenerationIsNeverAborted(t *testing.T) {
	generator := newFakeKoboldGenerator()
	close(generator.release)
	service, runtime := koboldForwardFixture(t, generator)
	clientContext, leave := context.WithCancel(context.Background())

	response, err := forwardChat(service, runtime, clientContext, `{"model":"m","messages":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	leave()

	select {
	case aborted := <-generator.aborted:
		t.Fatalf("a delivered generation was aborted by key %q; the abort would hit whatever runs next", aborted)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestClientChosenGenkeyIsKeptAndUsedForTheAbort(t *testing.T) {
	generator := newFakeKoboldGenerator()
	service, runtime := koboldForwardFixture(t, generator)
	clientContext, leave := context.WithCancel(context.Background())
	go func() {
		_, _ = forwardChat(service, runtime, clientContext, `{"model":"m","genkey":"client-key"}`)
	}()
	waitUntil(t, func() bool { return generator.generationKey() != "" })

	leave()

	if aborted := <-generator.aborted; aborted != "client-key" || generator.generationKey() != "client-key" {
		t.Fatalf("aborted %q, generation %q, want the client's own key on both", aborted, generator.generationKey())
	}
}

func TestOnlyKoboldGenerationRequestsGetAGenerationKey(t *testing.T) {
	body := []byte(`{"model":"m"}`)
	cases := []struct {
		name string
		path string
		mode string
		body []byte
		want bool
	}{
		{"kobold chat", "/v1/chat/completions", BackendModeKobold, body, true},
		{"kobold native stream", koboldNativeStreamPath, BackendModeKobold, body, true},
		{"kobold models listing", "/v1/models", BackendModeKobold, body, false},
		{"llama chat", "/v1/chat/completions", BackendModeLlamaSDCPP, body, false},
		{"kobold chat with a body that is not a json object", "/v1/chat/completions", BackendModeKobold, []byte(`[1]`), false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			marked, key := markKoboldGeneration(testCase.path, testCase.mode, testCase.body)
			if (key != "") != testCase.want {
				t.Fatalf("key = %q, want a key: %t", key, testCase.want)
			}
			if !testCase.want && string(marked) != string(testCase.body) {
				t.Fatalf("body was rewritten for a request that cannot be aborted: %s", marked)
			}
		})
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}
