package cluster

import "testing"

func TestPeerHealthReportsEveryKnownPeer(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master.invalid")
	for _, nodeID := range []string{"slave-a", "slave-b"} {
		if err := registry.UpdateNode(Snapshot{ProtocolVersion: ProtocolVersion, NodeID: nodeID, NodeURL: "http://" + nodeID + ".invalid"}); err != nil {
			t.Fatal(err)
		}
	}
	registry.MarkNodeHealth("slave-b", false)

	health := registry.PeerHealth()
	if len(health) != 2 || !health["slave-a"] || health["slave-b"] {
		t.Fatalf("health=%v, want slave-a healthy and slave-b unhealthy", health)
	}
}
