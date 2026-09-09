package routerstoretest

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"

	"tensors-router/internal/routerstore"
)

func Open(t *testing.T, modules ...routerstore.Module) *routerstore.Handle {
	t.Helper()
	return OpenAt(t, filepath.Join(t.TempDir(), "analytics.sqlite"), modules...)
}

func OpenAt(t *testing.T, path string, modules ...routerstore.Module) *routerstore.Handle {
	t.Helper()
	return OpenConfig(t, routerstore.Config{Path: path, Modules: modules})
}

func OpenConfig(t *testing.T, config routerstore.Config) *routerstore.Handle {
	t.Helper()
	if config.Logger == nil {
		config.Logger = log.New(io.Discard, "", 0)
	}
	handle, err := routerstore.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("open router store: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return handle
}
