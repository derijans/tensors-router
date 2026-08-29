package cluster

import "testing"

type fakeGroupSource struct {
	groupID string
	members []GroupMember
}

func (source *fakeGroupSource) GroupMembers(member GroupMember) (string, []GroupMember, bool) {
	for _, candidate := range source.members {
		if candidate == member {
			return source.groupID, source.members, true
		}
	}
	return "", nil, false
}

// fakeCostSource states each node's per-job cost, its queued backlog, and its
// model load cost directly, so every test says exactly which node it expects to
// win and why. A node missing from perJobMS is unqualified.
type fakeCostSource struct {
	perJobMS map[string]float64
	loadMS   map[string]float64
	backlog  map[string]int64
}

func (source *fakeCostSource) PredictMS(nodeID string, modelID string, lane string, work float64) (float64, bool) {
	perJob, ok := source.perJobMS[nodeID]
	if !ok {
		return 0, false
	}
	return perJob, true
}

func (source *fakeCostSource) PredictQueueMS(nodeID string, modelID string, lane string, count int64, work float64) (float64, bool) {
	perJob, ok := source.perJobMS[nodeID]
	if !ok {
		return 0, false
	}
	return float64(count) * perJob, true
}

func (source *fakeCostSource) SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool) {
	load, ok := source.loadMS[nodeID]
	return load, ok
}

func (source *fakeCostSource) NodeBacklog(nodeID string, groupID string) (int64, float64) {
	return source.backlog[nodeID], float64(source.backlog[nodeID])
}

func imageModel(id string, nodeID string, modelHash string, configHash string, source string) Model {
	model := testModel(id, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasImage = true
	model.ImageID = id
	model.PublicImageID = id
	model.BackendMode = BackendModeLlamaSDCPP
	return model
}

// Two nodes hold the same checkpoint under different config names, so their
// config hashes differ and the registry forks them into separate public IDs.
// Without a group they can never share traffic.
func newForkedImageRegistry(t *testing.T, loadedOnSlave bool) *Registry {
	t.Helper()
	registry := NewRegistry(RoleMaster, "master", "http://master")
	local := imageModel("sdxl", "master", "weights", "config-a", SourceMaster)
	local.Loaded = true
	if err := registry.UpdateLocal([]Model{local}); err != nil {
		t.Fatal(err)
	}
	remote := imageModel("sdxl-alt", "slave-a", "weights", "config-b", SourceSlave)
	remote.Loaded = loadedOnSlave
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave-a", Models: []Model{remote}}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func groupOfForkedModels() *fakeGroupSource {
	return &fakeGroupSource{
		groupID: "group",
		members: []GroupMember{
			{NodeID: "master", ImageID: "sdxl"},
			{NodeID: "slave-a", ImageID: "sdxl-alt"},
		},
	}
}

func TestGroupRoutesAcrossDifferentPublicImageIDs(t *testing.T) {
	registry := newForkedImageRegistry(t, true)
	registry.SetGroupSource(groupOfForkedModels())
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000, "slave-a": 8000},
		backlog:  map[string]int64{"master": 12},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route for a grouped image model")
	}
	defer release()

	if route.NodeID != "slave-a" || !route.Remote {
		t.Fatalf("route = %+v, want the idle member on the other node", route)
	}
	if route.LocalImageID != "sdxl-alt" {
		t.Fatalf("local image id = %q, want the member's own id", route.LocalImageID)
	}
	// Responses are rewritten back to this value, so it has to stay the id the
	// client asked for rather than the member it happened to reach.
	if route.PublicImageID != "sdxl" {
		t.Fatalf("public image id = %q, want the requested id", route.PublicImageID)
	}
}

// The busy node holds the model and the idle one does not. With a deep backlog
// the load is still worth paying.
func TestGroupPrefersAnIdleMemberThatMustLoadWhenTheBacklogIsDeep(t *testing.T) {
	registry := newForkedImageRegistry(t, false)
	registry.SetGroupSource(groupOfForkedModels())
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000, "slave-a": 8000},
		loadMS:   map[string]float64{"slave-a": 19000},
		backlog:  map[string]int64{"master": 12},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route for a grouped image model")
	}
	defer release()
	if route.NodeID != "slave-a" {
		t.Fatalf("route = %+v, want the idle member despite its model load", route)
	}
}

// The same pair with a shallow backlog. Nothing about the nodes changed, only the
// queue, which is what proves the load is weighed rather than discounted.
func TestGroupKeepsTheLoadedMemberWhenTheLoadOutlastsTheBacklog(t *testing.T) {
	registry := newForkedImageRegistry(t, false)
	registry.SetGroupSource(groupOfForkedModels())
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000, "slave-a": 8000},
		loadMS:   map[string]float64{"slave-a": 19000},
		backlog:  map[string]int64{"master": 1},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route for a grouped image model")
	}
	defer release()
	if route.NodeID != "master" {
		t.Fatalf("route = %+v, want the loaded member", route)
	}
}

// An unmeasured member is never scheduled on a guess, and the whole group falls
// back so the existing rotation keeps building that member's history.
func TestGroupFallsBackToRotationWhileAMemberIsUnqualified(t *testing.T) {
	registry := newForkedImageRegistry(t, true)
	registry.SetGroupSource(groupOfForkedModels())
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000},
		backlog:  map[string]int64{"master": 50},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route for a grouped image model")
	}
	defer release()
	// The existing cascade prefers an idle local replica, which is what it would
	// have done before groups existed.
	if route.NodeID != "master" || route.Remote {
		t.Fatalf("route = %+v, want the untouched local-first cascade", route)
	}
}

func TestGroupFallsBackWhenTheRequestCarriesNoSize(t *testing.T) {
	registry := newForkedImageRegistry(t, true)
	registry.SetGroupSource(groupOfForkedModels())
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000, "slave-a": 8000},
		backlog:  map[string]int64{"master": 50},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{})
	if !ok {
		t.Fatal("no route for a grouped image model")
	}
	defer release()
	if route.NodeID != "master" || route.Remote {
		t.Fatalf("route = %+v, want the untouched cascade for an unpriced request", route)
	}
}

// A model nobody grouped must reach exactly the replicas it always could, and no
// others.
func TestUngroupedImageModelSeesOnlyItsOwnReplicas(t *testing.T) {
	registry := newForkedImageRegistry(t, true)
	registry.SetGroupSource(&fakeGroupSource{})
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000, "slave-a": 1},
		backlog:  map[string]int64{"master": 99},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route for an ungrouped image model")
	}
	defer release()
	if route.NodeID != "master" || route.LocalImageID != "sdxl" {
		t.Fatalf("route = %+v, want only the requested model on its own node", route)
	}
}

func TestGroupSelectionIsSkippedWithoutACostSource(t *testing.T) {
	registry := newForkedImageRegistry(t, true)
	registry.SetGroupSource(groupOfForkedModels())

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route without a cost source")
	}
	defer release()
	if route.NodeID != "master" {
		t.Fatalf("route = %+v, want the local-first cascade", route)
	}
}

// A member whose load cost has never been measured cannot be priced, so the group
// falls back rather than assuming the switch is free.
func TestGroupFallsBackWhenAMemberSwitchIsUnpriced(t *testing.T) {
	registry := newForkedImageRegistry(t, false)
	registry.SetGroupSource(groupOfForkedModels())
	registry.SetCostSource(&fakeCostSource{
		perJobMS: map[string]float64{"master": 8000, "slave-a": 8000},
		backlog:  map[string]int64{"master": 50},
	})

	route, release, ok := registry.AcquireImage("sdxl", true, "*", RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route for a grouped image model")
	}
	defer release()
	if route.NodeID != "master" {
		t.Fatalf("route = %+v, want the fallback cascade", route)
	}
}
