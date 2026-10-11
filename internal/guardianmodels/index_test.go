package guardianmodels

import "testing"

func TestGuardianComesFromTheMostSpecificScope(t *testing.T) {
	cluster := Endpoint{NodeID: "master", ModelID: "cluster-vision"}
	node := Endpoint{NodeID: "slave", ModelID: "node-vision"}
	model := Endpoint{NodeID: "master", ModelID: "model-vision"}
	index := NewIndex([]Assignment{
		{Scope: ClusterScope, Guardian: cluster},
		{Scope: NodeScope, NodeID: "slave", Guardian: node},
		{Scope: ModelScope, NodeID: "slave", ModelID: "qwen", Guardian: model},
	})

	cases := []struct {
		name    string
		node    string
		model   string
		want    Endpoint
		wantHit bool
	}{
		{"model scope wins", "slave", "qwen", model, true},
		{"node scope covers its other models", "slave", "llama", node, true},
		{"cluster scope covers other nodes", "master", "anything", cluster, true},
	}
	for _, tc := range cases {
		got, found := index.Resolve(tc.node, tc.model)
		if found != tc.wantHit || got != tc.want {
			t.Fatalf("%s: resolved %v (found %v), want %v", tc.name, got, found, tc.want)
		}
	}
}

func TestNoGuardianIsResolvedWhenNothingIsAssigned(t *testing.T) {
	if _, found := NewIndex(nil).Resolve("master", "qwen"); found {
		t.Fatal("resolved a guardian from an empty index")
	}
}

func TestGuardianModelIsNeverItsOwnGuardian(t *testing.T) {
	vision := Endpoint{NodeID: "master", ModelID: "gemma-vision"}
	index := NewIndex([]Assignment{{Scope: ClusterScope, Guardian: vision}})

	if _, found := index.Resolve("master", "gemma-vision"); found {
		t.Fatal("the guardian was given itself as guardian")
	}
}
