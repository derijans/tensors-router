package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/hardware"
	"tensors-router/internal/memoryfit"
	"tensors-router/internal/memorysamples"
	"tensors-router/internal/routerstore/routerstoretest"
	"tensors-router/internal/siteapi"
)

type growingGPUMemory struct {
	mu        sync.Mutex
	usedMB    int64
	totalMB   int64
	ramUsedMB int64
}

func (memory *growingGPUMemory) setUsed(usedMB int64) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.usedMB = usedMB
}

func (memory *growingGPUMemory) reading() hardware.MemoryReading {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return hardware.MemoryReading{Kind: hardware.MemoryKindVRAM, UsedMB: memory.usedMB, TotalMB: memory.totalMB, SampledAt: time.Now()}
}

func (memory *growingGPUMemory) Memory(context.Context) (hardware.MemoryReading, bool) {
	return memory.reading(), true
}

func (memory *growingGPUMemory) FreshMemory(context.Context) (hardware.MemoryReading, bool) {
	return memory.reading(), true
}

func (memory *growingGPUMemory) NodeMemory(context.Context) (hardware.NodeMemorySnapshot, bool) {
	reading := memory.reading()
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return hardware.NodeMemorySnapshot{
		Devices:  []hardware.GPUMemoryDevice{{Vendor: hardware.GPUVendorNVIDIA, Name: "RTX", DedicatedUsedMB: reading.UsedMB, DedicatedTotalMB: reading.TotalMB, UsageKnown: true}},
		RAM:      hardware.RAMInfo{UsedMB: memory.ramUsedMB, TotalMB: 32768},
		RAMKnown: true,
	}, true
}

func newMemoryFitTestService(t *testing.T, configs map[string]string, memory *growingGPUMemory) (*Service, *fakeBackend, *memorysamples.Store) {
	t.Helper()
	dir := t.TempDir()
	modelDir := t.TempDir()
	for _, name := range []string{"a.gguf", "b.gguf"} {
		if err := os.WriteFile(filepath.Join(modelDir, name), []byte("weights "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for id, content := range configs {
		writeProxyTestConfig(t, dir, id, strings.ReplaceAll(content, "{{models}}", filepath.ToSlash(modelDir)))
	}
	handle := routerstoretest.Open(t, memorysamples.SchemaModule{})
	store, err := memorysamples.NewStore(handle.DB(), handle.Reader())
	if err != nil {
		t.Fatal(err)
	}
	backend := newReadyFakeBackend(t, nil)
	service := NewService(ServiceConfig{
		Backend: backend, Catalog: catalog.New(dir), ConfigDir: dir, NodeID: "node-a",
		MemorySource: memory, MemorySampleStore: store, Logger: log.New(io.Discard, "", 0),
	})
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	return service, backend, store
}

func loadGrowingBy(t *testing.T, service *Service, backend *fakeBackend, memory *growingGPUMemory, modelID string, growthMB int64) {
	t.Helper()
	backend.onReload = func(string) { memory.setUsed(memory.reading().UsedMB + growthMB) }
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, modelID)
	service.memoryFit.recording.Wait()
}

func fitFor(t *testing.T, service *Service, modelID string) siteapi.ConfigMemoryFit {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/nodes/memory-fit?node_id=node-a", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("memory fit status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var fit siteapi.NodeMemoryFit
	if err := json.Unmarshal(recorder.Body.Bytes(), &fit); err != nil {
		t.Fatal(err)
	}
	for _, config := range fit.Configs {
		if config.ModelID == modelID {
			return config
		}
	}
	t.Fatalf("no fit for %q in %+v", modelID, fit)
	return siteapi.ConfigMemoryFit{}
}

func TestMemoryFitIsUnknownUntilAConfigHasLoaded(t *testing.T) {
	memory := &growingGPUMemory{usedMB: 500, totalMB: 24576}
	service, _, _ := newMemoryFitTestService(t, map[string]string{"model-a": `{"model_param":"{{models}}/a.gguf","gpulayers":99}`}, memory)

	if fit := fitFor(t, service, "model-a"); fit.Level != string(memoryfit.LevelUnknown) || fit.Tier != string(memoryfit.TierUnknown) {
		t.Fatalf("fit=%+v, want unknown before any load", fit)
	}
}

func TestMemoryFitLearnsFromALoadIntoAnEmptyRuntime(t *testing.T) {
	memory := &growingGPUMemory{usedMB: 500, totalMB: 24576}
	service, backend, _ := newMemoryFitTestService(t, map[string]string{"model-a": `{"model_param":"{{models}}/a.gguf","gpulayers":99}`}, memory)
	loadGrowingBy(t, service, backend, memory, "model-a", 6000)

	fit := fitFor(t, service, "model-a")
	if fit.Tier != string(memoryfit.TierMeasured) || fit.VRAM.LowMB != 6000 || fit.VRAM.HighMB != 6000 {
		t.Fatalf("fit=%+v, want the measured 6000 MB", fit)
	}
	if fit.Level != string(memoryfit.LevelGreen) || fit.GPUFreeMB != 24576-6500+6000 {
		t.Fatalf("fit=%+v, want green with the loaded model's own memory counted as free", fit)
	}
}

func TestMemoryFitSkipsALoadThatReplacedAnotherConfig(t *testing.T) {
	memory := &growingGPUMemory{usedMB: 500, totalMB: 24576}
	service, backend, store := newMemoryFitTestService(t, map[string]string{
		"model-a": `{"model_param":"{{models}}/a.gguf","gpulayers":99}`,
		"model-b": `{"model_param":"{{models}}/b.gguf","gpulayers":99}`,
	}, memory)
	loadGrowingBy(t, service, backend, memory, "model-a", 6000)
	loadGrowingBy(t, service, backend, memory, "model-b", 1000)

	target, _ := service.memoryFitTarget("model-b.kcpps")
	input, err := store.EstimateInput(context.Background(), target.combo, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Exact) != 0 {
		t.Fatalf("a switch from model-a recorded %+v as model-b's footprint", input.Exact)
	}
}

func TestMemoryFitSkipsALoadDisturbedByAnotherRuntime(t *testing.T) {
	service, _, _ := newMemoryFitTestService(t, map[string]string{"model-a": `{"model_param":"{{models}}/a.gguf"}`}, &growingGPUMemory{totalMB: 1})
	window := service.memoryFit.windows.begin()
	service.onRuntimeChanged()
	if service.memoryFit.windows.finish(window) {
		t.Fatal("a runtime change during the load left the window clean")
	}
	first := service.memoryFit.windows.begin()
	second := service.memoryFit.windows.begin()
	if service.memoryFit.windows.finish(first) || service.memoryFit.windows.finish(second) {
		t.Fatal("two overlapping loads were both treated as clean")
	}
}

func TestMemoryFitShowsRedWithoutBlockingTheLoad(t *testing.T) {
	memory := &growingGPUMemory{usedMB: 500, totalMB: 24576}
	service, backend, store := newMemoryFitTestService(t, map[string]string{"model-a": `{"model_param":"{{models}}/a.gguf","gpulayers":99}`}, memory)
	target, _ := service.memoryFitTarget("model-a.kcpps")
	if err := store.Record(context.Background(), memorysamples.Record{Combo: target.combo, VRAMMB: 90000, RAMMB: 0}); err != nil {
		t.Fatal(err)
	}
	if fit := fitFor(t, service, "model-a"); fit.Level != string(memoryfit.LevelRed) {
		t.Fatalf("fit=%+v, want red", fit)
	}
	reloadsBefore := backend.reloads.Load()
	loadGrowingBy(t, service, backend, memory, "model-a", 100)
	if backend.reloads.Load() == reloadsBefore {
		t.Fatal("a red verdict stopped the load")
	}
}

func TestMemoryFitTargetMapsKoboldQuantKVAndUnsetLayers(t *testing.T) {
	service, _, _ := newMemoryFitTestService(t, map[string]string{
		"model-a": `{"model_param":"{{models}}/a.gguf","quantkv":"1","contextsize":8192}`,
		"model-b": `{"model_param":"{{models}}/b.gguf","gpulayers":0}`,
	}, &growingGPUMemory{totalMB: 1})
	quantized, _ := service.memoryFitTarget("model-a.kcpps")
	if quantized.combo.KVTypeK != "q8_0" || quantized.combo.GPULayers != unknownGPULayers || quantized.combo.ContextSize != 8192 {
		t.Fatalf("combo=%+v", quantized.combo)
	}
	cpuOnly, _ := service.memoryFitTarget("model-b.kcpps")
	if !cpuOnly.ramOnly {
		t.Fatal("gpulayers 0 must be judged against RAM only")
	}
}

func TestMemoryFitTreatsConfigsOfOneModelFileAsOneModel(t *testing.T) {
	service, _, _ := newMemoryFitTestService(t, map[string]string{
		"short": `{"model_param":"{{models}}/a.gguf","gpulayers":99,"contextsize":2048}`,
		"long":  `{"model_param":"{{models}}/a.gguf","gpulayers":99,"contextsize":8192}`,
		"other": `{"model_param":"{{models}}/b.gguf","gpulayers":99,"contextsize":2048}`,
	}, &growingGPUMemory{totalMB: 1})
	short, _ := service.memoryFitTarget("short.kcpps")
	long, _ := service.memoryFitTarget("long.kcpps")
	other, _ := service.memoryFitTarget("other.kcpps")
	if !short.combo.Learnable() || short.combo.PlacementKey() != long.combo.PlacementKey() {
		t.Fatal("two context sizes of one model file must share a placement")
	}
	if short.combo.PlacementKey() == other.combo.PlacementKey() {
		t.Fatal("different model files must not share a placement")
	}
}

func TestMemoryFitLearnsNothingWithoutTheModelFile(t *testing.T) {
	service, _, _ := newMemoryFitTestService(t, map[string]string{"gone": `{"model_param":"{{models}}/missing.gguf"}`}, &growingGPUMemory{totalMB: 1})
	target, _ := service.memoryFitTarget("gone.kcpps")
	if target.combo.Learnable() {
		t.Fatal("a config whose model file is missing produced a learnable identity")
	}
}
