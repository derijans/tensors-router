package routinggroups

import (
	"context"
	"testing"
)

func textAnchorA() TextMember { return TextMember{NodeID: "node-a", ModelID: "llama-70b"} }
func textMemberB() TextMember { return TextMember{NodeID: "node-b", ModelID: "llama-70b-q8"} }
func textMemberC() TextMember { return TextMember{NodeID: "node-c", ModelID: "llama-70b-awq"} }

func TestSetTextGroupReplacesPreviousMembership(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.SetTextGroup(ctx, textAnchorA(), []TextMember{textMemberB()}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.SetTextGroup(ctx, textAnchorA(), []TextMember{textMemberC()})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Members) != 2 {
		t.Fatalf("saved members = %d, want 2", len(saved.Members))
	}
	group, found, err := store.TextGroup(ctx, textMemberB())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("member replaced out of the group is still reported in one: %+v", group)
	}
}

func TestTextGroupOfOneIsDeleted(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.SetTextGroup(ctx, textAnchorA(), []TextMember{textMemberB()}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.SetTextGroup(ctx, textAnchorA(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != "" {
		t.Fatalf("a group of one was not deleted: %+v", saved)
	}
	if _, found, err := store.TextGroup(ctx, textAnchorA()); err != nil || found {
		t.Fatalf("anchor left behind after its group was deleted: found=%t err=%v", found, err)
	}
}

// TestTextAndImageGroupsAreIndependent is what justifies two tables rather than
// a lane column on one: a model that is both an LLM and an image model must be
// able to sit in one group per lane, and editing one lane's membership must
// never touch the other's rows.
func TestTextAndImageGroupsAreIndependent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	shared := struct {
		NodeID string
		ID     string
	}{"node-a", "qwen-vl"}

	if _, err := store.SetGroup(ctx, Member{NodeID: shared.NodeID, ImageID: shared.ID}, []Member{{NodeID: "node-b", ImageID: "qwen-vl-alt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetTextGroup(ctx, TextMember{NodeID: shared.NodeID, ModelID: shared.ID}, []TextMember{{NodeID: "node-c", ModelID: "qwen-text-alt"}}); err != nil {
		t.Fatal(err)
	}

	imageGroup, found, err := store.Group(ctx, Member{NodeID: shared.NodeID, ImageID: shared.ID})
	if err != nil || !found {
		t.Fatalf("image group missing after both lanes set: found=%t err=%v", found, err)
	}
	if len(imageGroup.Members) != 2 {
		t.Fatalf("image group members = %d, want 2", len(imageGroup.Members))
	}
	textGroup, found, err := store.TextGroup(ctx, TextMember{NodeID: shared.NodeID, ModelID: shared.ID})
	if err != nil || !found {
		t.Fatalf("text group missing after both lanes set: found=%t err=%v", found, err)
	}
	if len(textGroup.Members) != 2 {
		t.Fatalf("text group members = %d, want 2", len(textGroup.Members))
	}
}

func TestDeleteTextGroupLeavesImageGroupsIntact(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.SetGroup(ctx, anchorA(), []Member{memberB()}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetTextGroup(ctx, textAnchorA(), []TextMember{textMemberB()}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTextGroup(ctx, textAnchorA()); err != nil {
		t.Fatal(err)
	}
	group, found, err := store.Group(ctx, anchorA())
	if err != nil || !found {
		t.Fatalf("image group disturbed by an unrelated text group deletion: found=%t err=%v", found, err)
	}
	if len(group.Members) != 2 {
		t.Fatalf("image group members = %d, want 2", len(group.Members))
	}
}
