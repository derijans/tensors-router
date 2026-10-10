package proxy

import (
	"context"
	"errors"
	"io"
	"log"
	"net/url"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/routerstore/routerstoretest"
)

func TestFailedModelLoadIsRecordedWithItsModelAndConfig(t *testing.T) {
	handle := routerstoretest.Open(t, loaderrors.SchemaModule{})
	store, err := loaderrors.NewStore(loaderrors.StoreConfig{NodeID: "node-a", DB: handle.DB(), ReadDB: handle.Reader()})
	if err != nil {
		t.Fatal(err)
	}
	backendURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true, reloadErr: func(string) error { return errors.New("model file is corrupt") }}
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "text", `{"model_param":"text.gguf"}`)
	service := NewService(ServiceConfig{
		Backend:        backend,
		Catalog:        catalog.New(dir),
		ConfigDir:      dir,
		LoadErrorStore: store,
		Logger:         log.New(io.Discard, "", 0),
	})

	_, _, _, err = service.acquireModelConfigForBackendMode(BackendModeKobold, context.Background(), "text-model", "text.kcpps", readinessText, false)
	if err == nil {
		t.Fatal("a reload that fails must fail the acquisition")
	}

	result, err := store.List(context.Background(), loaderrors.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("recorded %d load errors, want 1", len(result.Records))
	}
	record := result.Records[0]
	if record.ModelID != "text-model" || record.ConfigName != "text.kcpps" {
		t.Fatalf("load error names model %q config %q, want text-model / text.kcpps", record.ModelID, record.ConfigName)
	}
}
