package cluster

import "testing"

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

	imageRoute, releaseImage, ok := registry.AcquireImage("combo-dream", true, "*")
	if !ok || imageRoute.Remote || imageRoute.Lane != RouteLaneImage {
		t.Fatalf("expected local image route while text lane busy %#v ok=%t", imageRoute, ok)
	}
	releaseImage()
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
