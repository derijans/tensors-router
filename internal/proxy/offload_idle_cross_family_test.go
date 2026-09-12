package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/catalog"
)

func newTwoFamilyTestService(t *testing.T) *Service {
	t.Helper()
	koboldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(koboldServer.Close)
	koboldURL, err := url.Parse(koboldServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	llamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(llamaServer.Close)
	llamaURL, err := url.Parse(llamaServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	koboldBackend := &fakeBackend{url: koboldURL, healthy: true}
	llamaBackend := &fakeBackend{url: llamaURL, healthy: true}

	service := NewService(ServiceConfig{
		Backend:     koboldBackend,
		BackendMode: backendmode.Kobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			backendmode.Kobold: {
				TextBackend:       koboldBackend,
				EmbeddingsBackend: koboldBackend,
			},
			backendmode.LlamaSDCPP: {
				TextBackend:       llamaBackend,
				EmbeddingsBackend: llamaBackend,
			},
		},
		Catalog: catalog.New(t.TempDir()),
		Logger:  log.New(io.Discard, "", 0),
	})
	return service
}

func TestIdleForBorrowedWorkSeesActivityInAnyConfiguredFamily(t *testing.T) {
	service := newTwoFamilyTestService(t)
	if service.currentBackendMode() != backendmode.Kobold {
		t.Fatalf("current backend mode = %q, want kobold", service.currentBackendMode())
	}
	if !service.idleForBorrowedWork() {
		t.Fatal("a freshly constructed two-family node reported busy")
	}

	otherFamily := service.backendFamilies[backendmode.LlamaSDCPP]
	if otherFamily == nil || otherFamily.textRuntime == nil {
		t.Fatal("llama_sdcpp family was not constructed with a text runtime")
	}
	otherFamily.textRuntime.state.mu.Lock()
	otherFamily.textRuntime.state.users++
	otherFamily.textRuntime.state.mu.Unlock()

	if service.idleForBorrowedWork() {
		t.Fatal("activity on a non-current family's runtime was invisible to idleForBorrowedWork")
	}
}
