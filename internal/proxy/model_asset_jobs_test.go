package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/siteapi"
)

type blockedAssetLookup struct {
	started chan struct{}
	release func()
}

func startBlockedMasterAssetLookup(t *testing.T) (*httptest.Server, blockedAssetLookup) {
	t.Helper()
	started := make(chan struct{}, 1)
	released := make(chan struct{})
	var releaseOnce sync.Once
	lookup := blockedAssetLookup{started: started, release: func() { releaseOnce.Do(func() { close(released) }) }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-released
	}))
	t.Cleanup(server.Close)
	t.Cleanup(lookup.release)
	return server, lookup
}

func newServiceWithUnresolvedPortableConfig(t *testing.T, masterURL string) (*Service, *modelassets.Index) {
	t.Helper()
	root := t.TempDir()
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	content := `{"model_param_hash":"` + strings.Repeat("c", 64) + `","model_param_filename":"missing.gguf"}`
	if err := os.WriteFile(filepath.Join(root, "portable.kcpps"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		ClusterRole:   cluster.RoleSlave,
		MasterURL:     masterURL,
		ClusterClient: cluster.NewClient("", masterURL),
		Catalog:       catalog.New(root),
		ConfigDir:     root,
		AssetIndex:    index,
	})
	return service, index
}

func startBlockedModelAssetJob(t *testing.T) (*Service, *modelassets.Index, modelassets.ResolutionJob, blockedAssetLookup) {
	t.Helper()
	master, lookup := startBlockedMasterAssetLookup(t)
	service, index := newServiceWithUnresolvedPortableConfig(t, master.URL)
	job, err := service.assets.createLocalModelAssetJob(siteapi.ModelAssetConfigRequest{Filename: "portable.kcpps"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-lookup.started:
	case <-time.After(5 * time.Second):
		t.Fatal("model asset job never reached the peer lookup")
	}
	return service, index, job, lookup
}

func TestCloseWaitsForARunningModelAssetJob(t *testing.T) {
	service, index, job, lookup := startBlockedModelAssetJob(t)

	closed := make(chan error, 1)
	go func() { closed <- service.Close(context.Background()) }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned %v while a model asset job was still running", err)
	case <-time.After(100 * time.Millisecond):
	}
	lookup.release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close never returned after the model asset job finished")
	}

	persisted, found, err := index.ResolutionJob(job.ID)
	if err != nil || !found {
		t.Fatalf("resolution job lookup found=%t error=%v", found, err)
	}
	if persisted.State != modelassets.JobFailed && persisted.State != modelassets.JobCompleted {
		t.Fatalf("job state after Close = %q, want a finished job", persisted.State)
	}
}

func TestCloseStopsWaitingForAModelAssetJobAtItsDeadline(t *testing.T) {
	service, _, _, _ := startBlockedModelAssetJob(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := service.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want the close deadline", err)
	}
}

func TestModelAssetJobsAreRefusedAfterClose(t *testing.T) {
	service, _ := newServiceWithUnresolvedPortableConfig(t, "")
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := service.assets.createLocalModelAssetJob(siteapi.ModelAssetConfigRequest{Filename: "portable.kcpps"}); !errors.Is(err, errAssetManagerClosed) {
		t.Fatalf("job creation after Close error = %v, want %v", err, errAssetManagerClosed)
	}
}
