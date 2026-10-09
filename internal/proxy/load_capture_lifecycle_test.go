package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tensors-router/internal/loadcapture"
	"tensors-router/internal/routerstore/routerstoretest"
)

func TestConcurrentPreloadsCaptureLoadingSuccessAndReuse(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	store := enableLoadCaptureForTest(t, service)

	reloadStarted := make(chan struct{})
	continueReload := make(chan struct{})
	backend.onReload = func(string) {
		close(reloadStarted)
		<-continueReload
	}

	firstResult := make(chan error, 1)
	go func() { firstResult <- service.PreloadModel(context.Background(), "a") }()
	select {
	case <-reloadStarted:
	case err := <-firstResult:
		t.Fatalf("preload ended before backend reload: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("backend reload did not start")
	}

	attempts, err := store.ListFiltered(context.Background(), loadcapture.ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Kind != loadcapture.KindPhysical || attempts[0].Status != loadcapture.StatusLoading {
		t.Fatalf("physical attempt was not visible while loading: %#v", attempts)
	}

	secondResult := make(chan error, 1)
	go func() { secondResult <- service.PreloadModel(context.Background(), "a") }()
	close(continueReload)
	for index, result := range []<-chan error{firstResult, secondResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("preload %d failed: %v", index+1, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("preload %d did not finish", index+1)
		}
	}

	attempts, err = store.ListFiltered(context.Background(), loadcapture.ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if backend.reloads.Load() != 1 || len(attempts) != 2 {
		t.Fatalf("expected one physical load and one reuse: reloads=%d attempts=%#v", backend.reloads.Load(), attempts)
	}
	var physical loadcapture.Attempt
	var reuse loadcapture.Attempt
	for _, attempt := range attempts {
		if attempt.Kind == loadcapture.KindPhysical {
			physical = attempt
		} else if attempt.Kind == loadcapture.KindReuse {
			reuse = attempt
		}
	}
	if physical.Status != loadcapture.StatusSucceeded || reuse.Status != loadcapture.StatusReused || reuse.PhysicalAttemptID != physical.ID {
		t.Fatalf("unexpected terminal capture records: physical=%#v reuse=%#v", physical, reuse)
	}
}

func TestReloadFailureCompletesPhysicalCapture(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	store := enableLoadCaptureForTest(t, service)
	backend.reloadErr = func(string) error { return errors.New("reload denied") }

	if err := service.PreloadModel(context.Background(), "a"); err == nil {
		t.Fatal("expected preload failure")
	}
	attempts, err := store.ListFiltered(context.Background(), loadcapture.ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Status != loadcapture.StatusFailed || attempts[0].FailureMessage != "reload denied" {
		t.Fatalf("reload failure was not captured: %#v", attempts)
	}
}

type knownHashCatalog struct {
	ModelCatalog
	hashes map[string]string
}

func (catalog knownHashCatalog) HashFile(path string) (string, bool) {
	hash, known := catalog.hashes[path]
	return hash, known
}

func TestPhysicalCaptureUsesCatalogKnownModelHash(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	store := enableLoadCaptureForTest(t, service)
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]string{"model_param": model})
	if err != nil {
		t.Fatal(err)
	}
	writeProxyTestConfig(t, service.configDir, "a", string(config))
	const knownHash = "3333333333333333333333333333333333333333333333333333333333333333"
	service.catalog = knownHashCatalog{ModelCatalog: service.catalog, hashes: map[string]string{model: knownHash}}

	if err := service.PreloadModel(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	attempts, err := store.ListFiltered(context.Background(), loadcapture.ListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || len(attempts[0].ModelHashes) != 1 || attempts[0].ModelHashes[0] != "model_param:"+knownHash {
		t.Fatalf("physical capture must use the catalog's known model hash: %#v", attempts)
	}
}

func enableLoadCaptureForTest(t *testing.T, service *Service) *loadcapture.Store {
	t.Helper()
	configDir := t.TempDir()
	writeProxyTestConfig(t, configDir, "a", "{}")
	handle := routerstoretest.Open(t, loadcapture.SchemaModule{})
	store, err := loadcapture.NewStore(loadcapture.StoreConfig{NodeID: "local", DB: handle.DB(), ReadDB: handle.Reader()})
	if err != nil {
		t.Fatal(err)
	}
	service.configDir = configDir
	service.loadCaptureStore = store
	service.loadCaptureMaxOutputBytes = 1024
	return store
}
