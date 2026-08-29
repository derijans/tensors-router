package cluster

import (
	"reflect"
	"testing"
)

func TestRegistryRetainsNodeURLWithoutModels(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(Snapshot{NodeID: "files-only", NodeURL: "http://files-only"}); err != nil {
		t.Fatal(err)
	}
	urls := registry.NodeURLsByID()
	if urls["master"] != "http://master" || urls["files-only"] != "http://files-only" {
		t.Fatalf("unexpected node urls %#v", urls)
	}
}

func TestRegistryExcludesDisabledReplicasAndUsesEnabledFallback(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	local := testModel("shared", "master", "hash", "config", SourceMaster)
	local.HasEmbeddings = true
	local.HasImage = true
	local.ImageID = "shared-image"
	local.PublicImageID = "shared-image"
	local.HasVoice = true
	local.HasMusic = true
	local.BackendMode = BackendModeLlamaSDCPP
	local.Disabled = true
	if err := registry.UpdateLocal([]Model{local}); err != nil {
		t.Fatal(err)
	}
	remote := testModel("shared", "slave-a", "hash", "config", SourceSlave)
	remote.HasEmbeddings = true
	remote.HasImage = true
	remote.ImageID = "shared-image"
	remote.PublicImageID = "shared-image"
	remote.HasVoice = true
	remote.HasMusic = true
	remote.BackendMode = BackendModeLlamaSDCPP
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave-a", Models: []Model{remote}}); err != nil {
		t.Fatal(err)
	}
	route, release, ok := registry.Acquire("shared", true)
	if !ok || route.NodeID != "slave-a" || !route.Remote {
		t.Fatalf("expected enabled remote fallback, route=%#v ok=%t", route, ok)
	}
	release()
	if models := PublicCatalogModels(registry.Models()); len(models) != 1 || models[0].ID != "shared" {
		t.Fatalf("enabled replica missing from public catalog %#v", models)
	}
	remote.Disabled = true
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave-a", Models: []Model{remote}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := registry.Acquire("shared", true); ok {
		t.Fatal("disabled replicas remained routable")
	}
	if _, _, ok := registry.AcquireEmbedding("shared", true); ok {
		t.Fatal("disabled embedding replicas remained routable")
	}
	if _, _, ok := registry.AcquireImage("shared-image", true, "*", RouteHint{}); ok {
		t.Fatal("disabled image replicas remained routable")
	}
	if _, _, ok := registry.AcquireVoice("shared", true); ok {
		t.Fatal("disabled voice replicas remained routable")
	}
	if _, _, ok := registry.AcquireMusic("shared", true); ok {
		t.Fatal("disabled music replicas remained routable")
	}
	if models := PublicCatalogModels(registry.Models()); len(models) != 0 {
		t.Fatalf("disabled replicas remained public %#v", models)
	}
}

func TestRegistryDedupeAndIndexesConflictingSlaveModel(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]Model{testModel("same", "master", "mhash", "chash", SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(Snapshot{
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models: []Model{
			testModel("same", "slave-a", "mhash", "chash", SourceSlave),
			testModel("same", "slave-a", "other", "chash", SourceSlave),
		},
	}); err != nil {
		t.Fatal(err)
	}

	models := registry.Models()
	if countPublicID(models, "same") != 2 {
		t.Fatalf("expected master and identical slave under same public id: %#v", models)
	}
	if countPublicID(models, "same-2") != 1 {
		t.Fatalf("expected conflicting slave to be indexed: %#v", models)
	}

	public := PublicCatalogModels(models)
	if len(public) != 2 || public[0].ID != "same" || public[1].ID != "same-2" {
		t.Fatalf("unexpected public OpenAI models %#v", public)
	}
}

func TestRegistryPrefersMasterThenBalancesSlavesWhenMasterBusy(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]Model{testModel("llm", "master", "mhash", "chash", SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []string{"slave-a", "slave-b"} {
		if err := registry.UpdateNode(Snapshot{
			NodeID:  nodeID,
			NodeURL: "http://" + nodeID,
			Models:  []Model{testModel("llm", nodeID, "mhash", "chash", SourceSlave)},
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, releaseFirst, ok := registry.Acquire("llm", true)
	if !ok || first.Remote || first.NodeID != "master" {
		t.Fatalf("expected master first route %#v ok=%t", first, ok)
	}
	defer releaseFirst()

	second, releaseSecond, ok := registry.Acquire("llm", true)
	if !ok || !second.Remote || second.NodeID != "slave-a" {
		t.Fatalf("expected first slave while master busy %#v ok=%t", second, ok)
	}
	releaseSecond()

	third, releaseThird, ok := registry.Acquire("llm", true)
	if !ok || !third.Remote || third.NodeID != "slave-b" {
		t.Fatalf("expected second slave round robin %#v ok=%t", third, ok)
	}
	releaseThird()
}

func TestRegistrySpecificEmbeddingRequiresLoadedRemote(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	model := testModel("embed", "slave-a", "hash", "config", SourceSlave)
	model.HasLLM = false
	model.HasEmbeddings = true
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave-a", Models: []Model{model}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := registry.AcquireSpecificEmbedding("slave-a", model.Filename, model.LocalID, true); ok {
		t.Fatal("unloaded remote embedding was acquired")
	}

	model.EmbeddingsLoaded = true
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave-a", Models: []Model{model}}); err != nil {
		t.Fatal(err)
	}
	route, release, ok := registry.AcquireSpecificEmbedding("slave-a", model.Filename, model.LocalID, true)
	if !ok || route.NodeID != "slave-a" || !route.Remote {
		t.Fatalf("loaded remote embedding was not acquired route=%#v ok=%t", route, ok)
	}
	release()
}

func TestRegistryKeepsSplitImageLaneLocalWhenTextLaneBusy(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	model := testModel("combo", "master", "mhash", "chash", SourceMaster)
	model.HasImage = true
	model.ImageID = "combo-dream"
	model.PublicImageID = "combo-dream"
	model.BackendMode = BackendModeLlamaSDCPP
	if err := registry.UpdateLocal([]Model{model}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(Snapshot{
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models:  []Model{model},
	}); err != nil {
		t.Fatal(err)
	}

	textRoute, releaseText, ok := registry.Acquire("combo", true)
	if !ok || textRoute.Remote || textRoute.Lane != RouteLaneText {
		t.Fatalf("expected local text route %#v ok=%t", textRoute, ok)
	}
	defer releaseText()

	imageRoute, releaseImage, ok := registry.AcquireImage("combo-dream", true, "*", RouteHint{})
	if !ok || imageRoute.Remote || imageRoute.Lane != RouteLaneImage {
		t.Fatalf("expected local image route while text lane busy %#v ok=%t", imageRoute, ok)
	}
	releaseImage()
}

func TestRegistryAcquiresVoiceAndMusicLanes(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	model := testModel("audio", "master", "mhash", "chash", SourceMaster)
	model.HasLLM = false
	model.HasVoice = true
	model.HasMusic = true
	if err := registry.UpdateLocal([]Model{model}); err != nil {
		t.Fatal(err)
	}

	voiceRoute, releaseVoice, ok := registry.AcquireVoice("audio", true)
	if !ok || voiceRoute.Lane != RouteLaneVoice {
		t.Fatalf("expected voice route %#v ok=%t", voiceRoute, ok)
	}
	defer releaseVoice()

	musicRoute, releaseMusic, ok := registry.AcquireMusic("audio", true)
	if !ok || musicRoute.Lane != RouteLaneMusic {
		t.Fatalf("expected music route %#v ok=%t", musicRoute, ok)
	}
	releaseMusic()
}

func TestRegistryAcquiresExactVoiceReplica(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	local := testModel("audio", "master", "hash", "config", SourceMaster)
	local.HasVoice = true
	remote := testModel("audio", "slave-a", "hash", "config", SourceSlave)
	remote.HasVoice = true
	if err := registry.UpdateLocal([]Model{local}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave-a", Models: []Model{remote}}); err != nil {
		t.Fatal(err)
	}

	route, release, ok := registry.AcquireSpecificVoice("audio", "slave-a", "audio.kcpps", true)
	if !ok || !route.Remote || route.NodeID != "slave-a" || route.Filename != "audio.kcpps" {
		t.Fatalf("expected exact remote voice route %#v ok=%t", route, ok)
	}
	release()
	if _, _, ok := registry.AcquireSpecificVoice("audio", "missing", "audio.kcpps", true); ok {
		t.Fatal("unexpected route for missing node")
	}
}

func TestRegistryWebUIRoutePrefersLocalActiveRoute(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	route, release, ok := registry.AcquireWebUI("kobold-lite", []Route{
		{PublicID: "text", LocalID: "text", NodeID: "slave-a", NodeURL: "http://slave-a", Remote: true, Lane: RouteLaneText},
		{PublicID: "text", LocalID: "text", NodeID: "master", NodeURL: "http://master", Lane: RouteLaneText},
	})
	defer release()
	if !ok || route.Remote || route.NodeID != "master" {
		t.Fatalf("expected local webui route %#v ok=%t", route, ok)
	}
}

func TestRegistryWebUIRoutePrefersIdleSlaveThenFallsBackToBusy(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	routes := []Route{
		{PublicID: "text-a", LocalID: "text-a", NodeID: "slave-a", NodeURL: "http://slave-a", Remote: true, Lane: RouteLaneText},
		{PublicID: "text-b", LocalID: "text-b", NodeID: "slave-b", NodeURL: "http://slave-b", Remote: true, Lane: RouteLaneText},
	}

	first, releaseFirst, ok := registry.AcquireWebUI("kobold-lite", routes)
	if !ok || !first.Remote || first.NodeID != "slave-a" {
		t.Fatalf("expected first idle slave route %#v ok=%t", first, ok)
	}
	second, releaseSecond, ok := registry.AcquireWebUI("kobold-lite", routes)
	if !ok || !second.Remote || second.NodeID != "slave-b" {
		t.Fatalf("expected second idle slave route %#v ok=%t", second, ok)
	}
	third, releaseThird, ok := registry.AcquireWebUI("kobold-lite", routes)
	if !ok || !third.Remote || (third.NodeID != "slave-a" && third.NodeID != "slave-b") {
		t.Fatalf("expected busy fallback route %#v ok=%t", third, ok)
	}
	releaseThird()
	releaseSecond()
	releaseFirst()
}

func TestRegistryMarksSlaveURLUnhealthy(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	for _, nodeID := range []string{"slave-a", "slave-b"} {
		if err := registry.UpdateNode(Snapshot{
			NodeID:  nodeID,
			NodeURL: "http://" + nodeID,
			Models:  []Model{testModel("llm", nodeID, "mhash", "chash", SourceSlave)},
		}); err != nil {
			t.Fatal(err)
		}
	}

	registry.MarkNodeURLHealth("http://slave-a", false)
	route, release, ok := registry.Acquire("llm", false)
	defer release()
	if !ok || route.NodeID != "slave-b" {
		t.Fatalf("expected healthy slave-b route %#v ok=%t", route, ok)
	}
}

func testModel(id string, nodeID string, modelHash string, configHash string, source string) Model {
	return Model{
		PublicID:   id,
		LocalID:    id,
		Filename:   id + ".kcpps",
		Created:    1,
		HasLLM:     true,
		ModelHash:  modelHash,
		ConfigHash: configHash,
		Source:     source,
		NodeID:     nodeID,
		NodeURL:    "http://" + nodeID,
		Available:  true,
	}
}

func countPublicID(models []Model, publicID string) int {
	count := 0
	for _, model := range models {
		if model.PublicID == publicID {
			count++
		}
	}
	return count
}

func TestRegistryRejectsDuplicateNodeIdentitiesWithoutMutation(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://MASTER:80/")
	if err := registry.UpdateLocal([]Model{testModel("local", "master", "hash", "config", SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://SLAVE:80/"}); err != nil {
		t.Fatal(err)
	}
	baselineRevision := registry.Revision()
	baselineModels := registry.Models()

	conflicts := []Snapshot{
		{NodeID: "master", NodeURL: "http://other"},
		{NodeID: "slave-master-url", NodeURL: "http://master"},
		{NodeID: "slave-a", NodeURL: "http://other"},
		{NodeID: "slave-b", NodeURL: "http://slave"},
	}
	for _, snapshot := range conflicts {
		err := registry.UpdateNode(snapshot)
		if ErrorCode(err) != ErrorCodeDuplicateNode {
			t.Fatalf("expected duplicate_node for %#v, got %v", snapshot, err)
		}
		if registry.Revision() != baselineRevision {
			t.Fatalf("rejected snapshot changed revision from %d to %d", baselineRevision, registry.Revision())
		}
		if !reflect.DeepEqual(registry.Models(), baselineModels) {
			t.Fatalf("rejected snapshot changed models")
		}
	}
}

func TestRegistryAcceptsNormalizedOwnerRefresh(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(Snapshot{NodeID: " slave-a ", NodeURL: "HTTP://SLAVE:80/"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(Snapshot{NodeID: "slave-a", NodeURL: "http://slave", Models: []Model{testModel("fresh", "slave-a", "hash", "config", SourceSlave)}}); err != nil {
		t.Fatal(err)
	}
	if models := registry.Models(); len(models) != 1 || models[0].LocalID != "fresh" {
		t.Fatalf("unexpected refreshed models %#v", models)
	}
}

func TestRegistryConcurrentIdentityClaimHasOneOwner(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, snapshot := range []Snapshot{{NodeID: "first", NodeURL: "http://shared"}, {NodeID: "second", NodeURL: "http://shared"}} {
		snapshot := snapshot
		go func() {
			<-start
			errors <- registry.UpdateNode(snapshot)
		}()
	}
	close(start)
	var successes int
	var duplicates int
	for range 2 {
		err := <-errors
		if err == nil {
			successes++
		} else if ErrorCode(err) == ErrorCodeDuplicateNode {
			duplicates++
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("unexpected claim results successes=%d duplicates=%d", successes, duplicates)
	}
	if len(registry.NodeURLsByID()) != 2 {
		t.Fatalf("unexpected owners %#v", registry.NodeURLsByID())
	}
}
