package offloadsettings

import (
	"context"
	"reflect"
	"testing"

	"tensors-router/internal/routerstore/routerstoretest"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	handle := routerstoretest.Open(t, SchemaModule{})
	return NewStore(handle.DB(), handle.Reader())
}

func TestStoreKeepsCanonicalOverridesUntilCleared(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.Set(ctx, Values{"offload_restore_delay": "1500ms", "scheduling_min_samples": "6"}); err != nil {
		t.Fatal(err)
	}
	overrides, err := store.Overrides(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Values{"offload_restore_delay": "1.5s", "scheduling_min_samples": "6"}); !reflect.DeepEqual(overrides, want) {
		t.Fatalf("overrides = %v, want %v", overrides, want)
	}

	if err := store.Clear(ctx, "scheduling_min_samples"); err != nil {
		t.Fatal(err)
	}
	if overrides, _ := store.Overrides(ctx); !reflect.DeepEqual(overrides, Values{"offload_restore_delay": "1.5s"}) {
		t.Fatalf("after clearing one key overrides = %v", overrides)
	}
	if err := store.ClearAll(ctx); err != nil {
		t.Fatal(err)
	}
	if overrides, _ := store.Overrides(ctx); len(overrides) != 0 {
		t.Fatalf("after clearing all overrides = %v", overrides)
	}
}

func TestStoreRejectsABatchWithOneInvalidValue(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.Set(ctx, Values{"offload_probe_idle": "3s", "scheduling_backend_depth": "0"}); err == nil {
		t.Fatal("stored a batch containing an invalid value")
	}
	if overrides, _ := store.Overrides(ctx); len(overrides) != 0 {
		t.Fatalf("overrides = %v, want nothing from the rejected batch", overrides)
	}
}
