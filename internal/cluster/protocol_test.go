package cluster

import (
	"strings"
	"testing"
)

func TestSnapshotWithoutAProtocolVersionIsRefused(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	err := registry.UpdateNode(Snapshot{NodeID: "old-slave", NodeURL: "http://old-slave"})
	if ErrorCode(err) != ErrorCodeIncompatibleProtocol {
		t.Fatalf("expected incompatible_protocol, got %v", err)
	}
	if _, ok := registry.NodeURLsByID()["old-slave"]; ok {
		t.Fatalf("refused node was registered")
	}
}

func TestPeerBelowTheMinimumIsRefused(t *testing.T) {
	if err := CompatiblePeer(MinimumProtocolVersion - 1); err == nil {
		t.Fatal("expected a peer below the minimum to be refused")
	}
}

func TestPeerAboveTheCurrentVersionIsAccepted(t *testing.T) {
	if err := CompatiblePeer(ProtocolVersion + 1); err != nil {
		t.Fatalf("expected a newer peer to be accepted, got %v", err)
	}
}

func TestRefusedNodeIsNotRegisteredAndCarriesAReason(t *testing.T) {
	registry := NewRegistry(RoleMaster, "master", "http://master")
	err := registry.UpdateNode(Snapshot{NodeID: "old-slave", NodeURL: "http://old-slave", ProtocolVersion: MinimumProtocolVersion - 1, BuildVersion: "v0.4.0"})
	if err == nil {
		t.Fatal("expected refusal")
	}
	if !strings.Contains(err.Error(), "old-slave") || !strings.Contains(err.Error(), "v0.4.0") {
		t.Fatalf("refusal reason missing node identity: %v", err)
	}
	if _, ok := registry.NodeURLsByID()["old-slave"]; ok {
		t.Fatalf("refused node was registered despite the error")
	}
}

func TestRegisterAnswersConflictNamingTheRequiredMinimum(t *testing.T) {
	err := CompatiblePeer(0)
	if err == nil || !strings.Contains(err.Error(), "0") {
		t.Fatalf("expected the peer's own version named in the error, got %v", err)
	}
	if !strings.Contains(err.Error(), "update the peer") {
		t.Fatalf("expected an update instruction, got %v", err)
	}
}

func TestSlaveRefusesAMasterBelowItsOwnMinimum(t *testing.T) {
	err := checkMasterCompatibility(RegisterResponse{OK: true, ProtocolVersion: MinimumProtocolVersion - 1}, "http://master", nil)
	if err == nil {
		t.Fatal("expected the slave to refuse an old master")
	}
	if err := checkMasterCompatibility(RegisterResponse{OK: true, ProtocolVersion: ProtocolVersion}, "http://master", nil); err != nil {
		t.Fatalf("expected a compatible master to be accepted, got %v", err)
	}
}
