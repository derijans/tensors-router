package cluster

import "testing"

func newMasterAndSlaveRegistry(t *testing.T, masterModel Model, slaveModel Model) *Registry {
	t.Helper()
	registry := NewRegistry(RoleMaster, "master", "http://master")
	masterModel.Loaded = true
	if err := registry.UpdateLocal([]Model{masterModel}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(Snapshot{ProtocolVersion: ProtocolVersion, NodeID: "slave", NodeURL: "http://slave", Models: []Model{slaveModel}}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func imageTestModel(id string, nodeID string, configHash string, source string) Model {
	model := testModel(id, nodeID, "ff-weights", configHash, source)
	model.HasLLM = false
	model.HasImage = true
	model.ImageID = id
	model.PublicImageID = id
	model.BackendMode = BackendModeLlamaSDCPP
	return model
}

func TestImageRequestIsServedByTheNodeHoldingTheRequestedID(t *testing.T) {
	registry := newMasterAndSlaveRegistry(t,
		imageTestModel("cc-ff", "master", "config-cc", SourceMaster),
		imageTestModel("cc11-ff", "slave", "config-cc11", SourceSlave))

	route, release, ok := registry.AcquireImage("cc11-ff", true, "*")
	if !ok {
		t.Fatal("no route for cc11-ff")
	}
	defer release()
	if route.NodeID != "slave" || !route.Remote || route.LocalImageID != "cc11-ff" {
		t.Fatalf("route = %+v, want the slave's own cc11-ff even though the master is local and idle", route)
	}
}

func TestTextRequestIsServedByTheNodeHoldingTheRequestedID(t *testing.T) {
	master := testModel("llama", "master", "weights", "config-a", SourceMaster)
	slave := testModel("llama-alt", "slave", "weights", "config-b", SourceSlave)
	registry := newMasterAndSlaveRegistry(t, master, slave)

	route, release, ok := registry.Acquire("llama-alt", true)
	if !ok {
		t.Fatal("no route for llama-alt")
	}
	defer release()
	if route.NodeID != "slave" || route.LocalID != "llama-alt" {
		t.Fatalf("route = %+v, want the slave's own llama-alt", route)
	}
}

func TestContextFitsRequiresAStatedWindowLargeEnough(t *testing.T) {
	model := testModel("llama", "master", "weights", "config", SourceMaster)
	model.Capabilities.Context = 4096
	if !ContextFits(model, 0) {
		t.Fatal("an unsized request must fit any model")
	}
	if !ContextFits(model, 4096) {
		t.Fatal("a request exactly the window size must fit")
	}
	if ContextFits(model, 4097) {
		t.Fatal("a request larger than the window was accepted")
	}
	model.Capabilities.Context = 0
	if ContextFits(model, 1) {
		t.Fatal("a model with no stated window was accepted for a sized request")
	}
}
