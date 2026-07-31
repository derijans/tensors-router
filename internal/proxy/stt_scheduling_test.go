package proxy

import (
	"testing"

	"tensors-router/internal/cluster"
)

func TestAutomaticSTTPriorityTiers(t *testing.T) {
	models := []cluster.Model{
		{PublicID: "local", Filename: "local.kcpps", NodeID: "master"},
		{PublicID: "remote-a", Filename: "a.kcpps", NodeID: "a"},
		{PublicID: "remote-b", Filename: "b.kcpps", NodeID: "b"},
	}
	service := &Service{nodeID: "master", clusterRole: cluster.RoleMaster}

	selected, ok := service.selectAutomaticSTTCandidate(models, NodeRuntimeStatus{ActiveSTTConfigFilename: "local.kcpps", ActiveRequests: 9}, map[string]NodeRuntimeStatus{
		"a": {ActiveSTTConfigFilename: "a.kcpps"},
	})
	if !ok || selected.PublicID != "local" {
		t.Fatalf("loaded local model did not win %#v ok=%t", selected, ok)
	}

	selected, ok = service.selectAutomaticSTTCandidate(models, NodeRuntimeStatus{ActiveRequests: 2}, map[string]NodeRuntimeStatus{
		"a": {ActiveSTTConfigFilename: "a.kcpps", ActiveRequests: 2, QueuedRequests: 1},
		"b": {ActiveSTTConfigFilename: "b.kcpps", ActiveRequests: 1, QueuedRequests: 1},
	})
	if !ok || selected.PublicID != "remote-b" {
		t.Fatalf("shortest loaded remote did not win %#v ok=%t", selected, ok)
	}

	selected, ok = service.selectAutomaticSTTCandidate(models, NodeRuntimeStatus{ActiveRequests: 2}, map[string]NodeRuntimeStatus{
		"a": {ActiveRequests: 0, QueuedRequests: 0},
		"b": {ActiveRequests: 1, QueuedRequests: 0},
	})
	if !ok || selected.PublicID != "remote-a" {
		t.Fatalf("whole-node idle candidate did not win %#v ok=%t", selected, ok)
	}

	selected, ok = service.selectAutomaticSTTCandidate(models, NodeRuntimeStatus{ActiveRequests: 2}, map[string]NodeRuntimeStatus{
		"a": {ActiveRequests: 2, QueuedRequests: 0},
		"b": {ActiveRequests: 1, QueuedRequests: 0},
	})
	if !ok || selected.PublicID != "local" {
		t.Fatalf("master queue tier did not win %#v ok=%t", selected, ok)
	}
}

func TestAutomaticSTTShortestQueueRoundRobinAndMissingStatus(t *testing.T) {
	models := []cluster.Model{
		{PublicID: "local", Filename: "local.kcpps", NodeID: "worker"},
		{PublicID: "remote-a", Filename: "a.kcpps", NodeID: "a"},
		{PublicID: "remote-b", Filename: "b.kcpps", NodeID: "b"},
		{PublicID: "legacy", Filename: "legacy.kcpps", NodeID: "legacy"},
	}
	service := &Service{nodeID: "worker", clusterRole: cluster.RoleSlave}
	statuses := map[string]NodeRuntimeStatus{
		"a": {ActiveRequests: 3, QueuedRequests: 2},
		"b": {ActiveRequests: 8, QueuedRequests: 2},
	}
	first, ok := service.selectAutomaticSTTCandidate(models, NodeRuntimeStatus{ActiveRequests: 4, QueuedRequests: 4}, statuses)
	second, secondOK := service.selectAutomaticSTTCandidate(models, NodeRuntimeStatus{ActiveRequests: 4, QueuedRequests: 4}, statuses)
	if !ok || !secondOK || first.PublicID != "remote-a" || second.PublicID != "remote-b" {
		t.Fatalf("unexpected shortest-queue rotation first=%#v second=%#v", first, second)
	}
}
