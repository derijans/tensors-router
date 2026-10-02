package proxy

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tensors-router/internal/hardware"
	"tensors-router/internal/siteapi"
)

type steppedMemorySource struct {
	mu         sync.Mutex
	kind       string
	usedByRead []int64
	reads      int
	latest     hardware.MemoryReading
}

func (source *steppedMemorySource) FreshMemory(context.Context) (hardware.MemoryReading, bool) {
	source.mu.Lock()
	defer source.mu.Unlock()
	used := source.usedByRead[min(source.reads, len(source.usedByRead)-1)]
	source.reads++
	source.latest = hardware.MemoryReading{Kind: source.kind, UsedMB: used, TotalMB: 24000, SampledAt: time.UnixMilli(1700000000000)}
	return source.latest, true
}

func (source *steppedMemorySource) Memory(context.Context) (hardware.MemoryReading, bool) {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.latest, source.reads > 0
}

func TestNodeStateReportsNodeMemoryAndTheFootprintOfEachLoad(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	koboldPath := filepath.Join(t.TempDir(), "koboldcpp")
	if err := os.WriteFile(koboldPath, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	service.backendBinaryPaths = map[string]string{backendIDKoboldCPP: koboldPath}
	service.nodeMemory = &steppedMemorySource{kind: hardware.MemoryKindRAM, usedByRead: []int64{1000, 5096}}
	ctx := context.Background()

	release, _, err := service.acquireModelConfig(defaultFamilyRuntime(t, service, readinessText), ctx, "a", "a.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	snapshot := service.localNodeState(ctx)
	want := siteapi.NodeMemory{Kind: hardware.MemoryKindRAM, TotalMB: 24000, UsedMB: 5096, SampledAtMS: 1700000000000}
	if snapshot.Memory == nil || *snapshot.Memory != want {
		t.Fatalf("expected node memory %#v, got %#v", want, snapshot.Memory)
	}
	row, found := loadedModelRow(snapshot, "a")
	if !found || row.MemoryEstimateMB != 4096 || row.Borrowed {
		t.Fatalf("expected a 4096 MB footprint for the loaded model, got %#v (found=%v)", row, found)
	}
}

func TestNodeStateOmitsMemoryWhenTheNodeCannotReadIt(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	service.nodeMemory = hardware.NewCachedMemorySource(nil, nil, time.Second)

	if snapshot := service.localNodeState(context.Background()); snapshot.Memory != nil {
		t.Fatalf("expected no memory reading, got %#v", snapshot.Memory)
	}
}

func loadedModelRow(snapshot siteapi.NodeState, modelID string) (siteapi.NodeStateModelRow, bool) {
	for _, backend := range snapshot.Backends {
		for _, row := range backend.LoadedModels {
			if row.ModelID == modelID {
				return row, true
			}
		}
	}
	return siteapi.NodeStateModelRow{}, false
}
