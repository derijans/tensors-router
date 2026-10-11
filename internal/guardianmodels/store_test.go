package guardianmodels

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

var visionOnMaster = Endpoint{NodeID: "master", ModelID: "gemma-vision"}

func TestStoreKeepsAssignmentsUntilCleared(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.Set(ctx, Assignment{Scope: ClusterScope, Guardian: visionOnMaster}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, Assignment{Scope: ModelScope, NodeID: "slave", ModelID: "qwen", Guardian: visionOnMaster}); err != nil {
		t.Fatal(err)
	}
	assignments, err := store.Assignments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Assignment{
		{Scope: ClusterScope, Guardian: visionOnMaster},
		{Scope: ModelScope, NodeID: "slave", ModelID: "qwen", Guardian: visionOnMaster},
	}
	if !reflect.DeepEqual(assignments, want) {
		t.Fatalf("assignments = %+v, want %+v", assignments, want)
	}

	if err := store.Clear(ctx, ModelScope, "slave", "qwen"); err != nil {
		t.Fatal(err)
	}
	if assignments, _ := store.Assignments(ctx); len(assignments) != 1 || assignments[0].Scope != ClusterScope {
		t.Fatalf("after clearing the model scope assignments = %+v", assignments)
	}
}

func TestSettingAScopeAgainReplacesItsGuardian(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	other := Endpoint{NodeID: "slave", ModelID: "other-vision"}

	_ = store.Set(ctx, Assignment{Scope: NodeScope, NodeID: "slave", Guardian: visionOnMaster})
	if err := store.Set(ctx, Assignment{Scope: NodeScope, NodeID: "slave", Guardian: other}); err != nil {
		t.Fatal(err)
	}

	assignments, _ := store.Assignments(ctx)
	if len(assignments) != 1 || assignments[0].Guardian != other {
		t.Fatalf("assignments = %+v, want the one node entry pointing at %v", assignments, other)
	}
}

func TestStoreRejectsAssignmentsThatNameNoTarget(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	rejected := []Assignment{
		{Scope: "galaxy", Guardian: visionOnMaster},
		{Scope: ClusterScope, Guardian: Endpoint{NodeID: "master"}},
		{Scope: NodeScope, Guardian: visionOnMaster},
		{Scope: ModelScope, NodeID: "slave", Guardian: visionOnMaster},
	}
	for _, assignment := range rejected {
		if err := store.Set(ctx, assignment); err == nil {
			t.Fatalf("stored %+v, want it rejected", assignment)
		}
	}
}
