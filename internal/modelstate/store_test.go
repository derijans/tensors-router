package modelstate

import (
	"context"
	"sync"
	"testing"
)

func TestStorePersistsIdempotentModelState(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if disabled, err := store.Disabled(ctx, "model-a"); err != nil || disabled {
		t.Fatalf("models must default enabled: disabled=%t err=%v", disabled, err)
	}
	if err := store.SetEnabled(ctx, "model-a", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEnabled(ctx, "model-a", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if disabled, err := reopened.Disabled(ctx, "model-a"); err != nil || !disabled {
		t.Fatalf("disabled state was not persisted: disabled=%t err=%v", disabled, err)
	}
	if err := reopened.SetEnabled(ctx, "model-a", true); err != nil {
		t.Fatal(err)
	}
	if err := reopened.SetEnabled(ctx, "model-a", true); err != nil {
		t.Fatal(err)
	}
	if disabled, err := reopened.Disabled(ctx, "model-a"); err != nil || disabled {
		t.Fatalf("enabled state was not restored: disabled=%t err=%v", disabled, err)
	}
}

func TestStoreSupportsConcurrentAccess(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	var workers sync.WaitGroup
	for index := 0; index < 16; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			localID := "model-a"
			if index%2 == 0 {
				localID = "model-b"
			}
			for iteration := 0; iteration < 20; iteration++ {
				if err := store.SetEnabled(ctx, localID, iteration%2 == 0); err != nil {
					t.Errorf("set enabled: %v", err)
					return
				}
				if _, err := store.Disabled(ctx, localID); err != nil {
					t.Errorf("read disabled: %v", err)
					return
				}
			}
		}(index)
	}
	workers.Wait()
}
