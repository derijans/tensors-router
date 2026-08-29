package routinggroups

import (
	"context"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return store
}

func anchorA() Member { return Member{NodeID: "node-a", ImageID: "sdxl-juggernautXL"} }
func memberB() Member { return Member{NodeID: "node-b", ImageID: "xl-jugg-q8"} }
func memberC() Member { return Member{NodeID: "node-c", ImageID: "juggernaut-xl-v9"} }

func TestSetGroupRoundTripsFromAnyMember(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	saved, err := store.SetGroup(ctx, anchorA(), []Member{memberB(), memberC()})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Members) != 3 {
		t.Fatalf("saved members = %d, want 3", len(saved.Members))
	}

	// Opening the dialog from any row has to reach the same group, otherwise a
	// second group could be created that overlaps the first.
	for _, member := range []Member{anchorA(), memberB(), memberC()} {
		group, found, err := store.Group(ctx, member)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("member %+v is not in any group", member)
		}
		if group.ID != saved.ID || len(group.Members) != 3 {
			t.Fatalf("member %+v resolved to %+v, want the saved group", member, group)
		}
	}
}

func TestGroupIsAbsentForUngroupedModels(t *testing.T) {
	store := newTestStore(t)
	if _, found, err := store.Group(context.Background(), anchorA()); err != nil || found {
		t.Fatalf("found=%t err=%v, want no group", found, err)
	}
}

// Members are keyed by the model they name, so joining a second group has to move
// them rather than duplicate them. Without that the router would have two
// candidate sets for one model and no way to pick between them.
func TestJoiningAnotherGroupMovesTheMember(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first, err := store.SetGroup(ctx, anchorA(), []Member{memberB()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SetGroup(ctx, memberC(), []Member{memberB()})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("expected two distinct groups")
	}

	group, found, err := store.Group(ctx, memberB())
	if err != nil {
		t.Fatal(err)
	}
	if !found || group.ID != second.ID {
		t.Fatalf("member resolved to %+v, want the second group", group)
	}

	// The first group is now down to its anchor alone and must not survive.
	if _, found, err := store.Group(ctx, anchorA()); err != nil || found {
		t.Fatalf("anchor still grouped after losing its only peer (found=%t err=%v)", found, err)
	}
	groups, err := store.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != second.ID {
		t.Fatalf("groups = %+v, want only the second", groups)
	}
}

func TestGroupOfOneIsNotStored(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	saved, err := store.SetGroup(ctx, anchorA(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != "" || len(saved.Members) != 0 {
		t.Fatalf("saved %+v, want nothing for a group with no peers", saved)
	}
	if _, found, err := store.Group(ctx, anchorA()); err != nil || found {
		t.Fatalf("found=%t err=%v, want no group", found, err)
	}
}

func TestSetGroupReplacesRatherThanAccumulates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.SetGroup(ctx, anchorA(), []Member{memberB(), memberC()}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.SetGroup(ctx, anchorA(), []Member{memberB()})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Members) != 2 {
		t.Fatalf("members = %+v, want the anchor and one peer", saved.Members)
	}
	if _, found, err := store.Group(ctx, memberC()); err != nil || found {
		t.Fatalf("removed member is still grouped (found=%t err=%v)", found, err)
	}
}

func TestDeleteGroupRemovesEveryMember(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.SetGroup(ctx, anchorA(), []Member{memberB(), memberC()}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteGroup(ctx, memberB()); err != nil {
		t.Fatal(err)
	}
	for _, member := range []Member{anchorA(), memberB(), memberC()} {
		if _, found, err := store.Group(ctx, member); err != nil || found {
			t.Fatalf("member %+v survived the delete (found=%t err=%v)", member, found, err)
		}
	}
	groups, err := store.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %+v, want none", groups)
	}
}

func TestSetGroupIgnoresDuplicatesAndBlankMembers(t *testing.T) {
	store := newTestStore(t)
	saved, err := store.SetGroup(context.Background(), anchorA(), []Member{
		memberB(),
		memberB(),
		{NodeID: "  node-b  ", ImageID: "  xl-jugg-q8  "},
		{NodeID: "", ImageID: "orphan"},
		{NodeID: "node-d", ImageID: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Members) != 2 {
		t.Fatalf("members = %+v, want the anchor and one distinct peer", saved.Members)
	}
}

func TestSetGroupRejectsABlankAnchor(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.SetGroup(context.Background(), Member{}, []Member{memberB()}); err == nil {
		t.Fatal("blank anchor was accepted")
	}
}

func TestGroupsSurviveReopeningTheStore(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetGroup(context.Background(), anchorA(), []Member{memberB()}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	group, found, err := reopened.Group(context.Background(), memberB())
	if err != nil || !found {
		t.Fatalf("found=%t err=%v, want the group to persist", found, err)
	}
	if len(group.Members) != 2 {
		t.Fatalf("members = %+v, want 2", group.Members)
	}
}
