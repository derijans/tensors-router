package routinggroups

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"tensors-router/internal/routerstore/routerstoretest"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	handle := routerstoretest.Open(t, SchemaModule{})
	return NewStore(handle.DB(), handle.Reader())
}

func masterFF() Endpoint { return Endpoint{NodeID: "master", ModelID: "cc-ff"} }
func slaveFF() Endpoint  { return Endpoint{NodeID: "slave", ModelID: "cc11-ff"} }
func thirdFF() Endpoint  { return Endpoint{NodeID: "third", ModelID: "ff-q8"} }

func lend(owner Endpoint, helper Endpoint) Link {
	return Link{Owner: owner, Helper: helper, LoadIfUnloaded: true}
}

func mustLinks(t *testing.T, store *Store, lane Lane) []Link {
	t.Helper()
	links, err := store.Links(context.Background(), lane)
	if err != nil {
		t.Fatal(err)
	}
	return links
}

func TestLinksKeepTheirDirectionAndFlags(t *testing.T) {
	store := newTestStore(t)
	onlyMasterLends := Link{Owner: masterFF(), Helper: slaveFF(), LoadIfUnloaded: false, RestoreAfterBorrow: true}
	if err := store.ReplaceLinksTouching(context.Background(), ImageLane, slaveFF(), []Link{onlyMasterLends}); err != nil {
		t.Fatal(err)
	}
	links := mustLinks(t, store, ImageLane)
	if !reflect.DeepEqual(links, []Link{onlyMasterLends}) {
		t.Fatalf("links = %+v, want only master lending to slave with its flags", links)
	}
}

func TestReplacingAnAnchorLeavesUnrelatedLinksAlone(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.ReplaceLinksTouching(ctx, ImageLane, masterFF(), []Link{lend(masterFF(), slaveFF()), lend(slaveFF(), masterFF())}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceLinksTouching(ctx, ImageLane, thirdFF(), []Link{lend(thirdFF(), slaveFF())}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceLinksTouching(ctx, ImageLane, masterFF(), []Link{lend(masterFF(), slaveFF())}); err != nil {
		t.Fatal(err)
	}
	want := []Link{lend(masterFF(), slaveFF()), lend(thirdFF(), slaveFF())}
	if links := mustLinks(t, store, ImageLane); !reflect.DeepEqual(links, want) {
		t.Fatalf("links = %+v, want %+v", links, want)
	}
}

func TestReplacingWithNoLinksUnlinksTheAnchor(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.ReplaceLinksTouching(ctx, ImageLane, masterFF(), []Link{lend(masterFF(), slaveFF()), lend(slaveFF(), masterFF())}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceLinksTouching(ctx, ImageLane, slaveFF(), nil); err != nil {
		t.Fatal(err)
	}
	if links := mustLinks(t, store, ImageLane); len(links) != 0 {
		t.Fatalf("links = %+v, want none", links)
	}
}

func TestLanesAreStoredSeparately(t *testing.T) {
	store := newTestStore(t)
	if err := store.ReplaceLinksTouching(context.Background(), TextLane, masterFF(), []Link{lend(masterFF(), slaveFF())}); err != nil {
		t.Fatal(err)
	}
	if links := mustLinks(t, store, ImageLane); len(links) != 0 {
		t.Fatalf("image links = %+v, want none", links)
	}
	if links := mustLinks(t, store, TextLane); len(links) != 1 {
		t.Fatalf("text links = %+v, want one", links)
	}
}

func TestReplaceRejectsLinksThatDoNotTouchTheAnchor(t *testing.T) {
	store := newTestStore(t)
	err := store.ReplaceLinksTouching(context.Background(), ImageLane, masterFF(), []Link{lend(thirdFF(), slaveFF())})
	if err == nil {
		t.Fatal("a link between two other models was accepted")
	}
}

func TestReplaceRejectsLendingWithinOneNode(t *testing.T) {
	store := newTestStore(t)
	sameNode := Endpoint{NodeID: "master", ModelID: "cc-ff-q4"}
	if err := store.ReplaceLinksTouching(context.Background(), ImageLane, masterFF(), []Link{lend(masterFF(), sameNode)}); err == nil {
		t.Fatal("a link inside one node was accepted")
	}
}

func TestReplaceRejectsABlankAnchor(t *testing.T) {
	store := newTestStore(t)
	if err := store.ReplaceLinksTouching(context.Background(), ImageLane, Endpoint{}, nil); err == nil {
		t.Fatal("blank anchor was accepted")
	}
}

func TestReplaceIgnoresDuplicatesAndBlankEndpoints(t *testing.T) {
	store := newTestStore(t)
	padded := Link{Owner: Endpoint{NodeID: " master ", ModelID: " cc-ff "}, Helper: Endpoint{NodeID: "slave", ModelID: "cc11-ff  "}, LoadIfUnloaded: true}
	err := store.ReplaceLinksTouching(context.Background(), ImageLane, masterFF(), []Link{
		lend(masterFF(), slaveFF()),
		padded,
		{Owner: masterFF(), Helper: Endpoint{NodeID: "", ModelID: "orphan"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if links := mustLinks(t, store, ImageLane); len(links) != 1 {
		t.Fatalf("links = %+v, want one distinct link", links)
	}
}

func TestLinksSurviveReopeningTheStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.sqlite")
	handle := routerstoretest.OpenAt(t, path, SchemaModule{})
	if err := NewStore(handle.DB(), handle.Reader()).ReplaceLinksTouching(context.Background(), ImageLane, masterFF(), []Link{lend(masterFF(), slaveFF())}); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := routerstoretest.OpenAt(t, path, SchemaModule{})
	if links := mustLinks(t, NewStore(reopened.DB(), reopened.Reader()), ImageLane); len(links) != 1 {
		t.Fatalf("links = %+v, want the saved link to persist", links)
	}
}
