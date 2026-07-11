package hardware

import (
	"context"
	"sync"
	"testing"
	"time"
)

type countingVRAMSource struct {
	mu    sync.Mutex
	calls int
}

func (source *countingVRAMSource) VRAM(context.Context) (VRAMInfo, bool) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.calls++
	return VRAMInfo{UsedMB: int64(source.calls), TotalMB: 100}, true
}

func (source *countingVRAMSource) count() int {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.calls
}

func TestVRAMSamplerSharesOneCadenceAcrossReaders(t *testing.T) {
	source := &countingVRAMSource{}
	sampler := NewVRAMSampler(source, 20*time.Millisecond)
	t.Cleanup(func() {
		if err := sampler.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	var readers sync.WaitGroup
	for range 32 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 100 {
				if _, ok := sampler.VRAM(context.Background()); !ok {
					t.Error("expected cached sample")
					return
				}
			}
		}()
	}
	readers.Wait()
	if calls := source.count(); calls != 1 {
		t.Fatalf("reader count affected source sampling: %d", calls)
	}
	time.Sleep(25 * time.Millisecond)
	if calls := source.count(); calls < 2 || calls > 3 {
		t.Fatalf("unexpected cadence source calls: %d", calls)
	}
}

func TestVRAMSamplerCloseStopsSampling(t *testing.T) {
	source := &countingVRAMSource{}
	sampler := NewVRAMSampler(source, 5*time.Millisecond)
	time.Sleep(8 * time.Millisecond)
	if err := sampler.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := source.count()
	time.Sleep(12 * time.Millisecond)
	if source.count() != calls {
		t.Fatal("sampler continued after close")
	}
}
