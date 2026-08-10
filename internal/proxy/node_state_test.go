package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/siteapi"
)

func TestLocalNodeStateDetectsRegularBinariesAndRedactsPaths(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	binaryDir := t.TempDir()
	koboldPath := filepath.Join(binaryDir, "koboldcpp-secret-location")
	if err := os.WriteFile(koboldPath, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	directoryPath := filepath.Join(binaryDir, "llama-directory")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	service.backendBinaryPaths = map[string]string{
		backendIDKoboldCPP:   koboldPath,
		backendIDLlamaServer: directoryPath,
		backendIDSDServer:    filepath.Join(binaryDir, "missing"),
	}
	state := service.textRuntime.state
	state.mu.Lock()
	state.modelID = "shared-model"
	state.filename = "shared.kcpps"
	state.generation = 9
	state.users = 2
	state.leases = map[uint64]string{11: "shared-model", 12: "shared-model"}
	state.mu.Unlock()

	snapshot := service.localNodeState()
	if len(snapshot.Backends) != 1 || snapshot.Backends[0].ID != backendIDKoboldCPP {
		t.Fatalf("unexpected detected backends %#v", snapshot.Backends)
	}
	rows := snapshot.Backends[0].LoadedModels
	if len(rows) != 1 || rows[0].ModelID != "shared-model" || rows[0].Lane != "text" || rows[0].Generation != 9 {
		t.Fatalf("unexpected shared runtime rows %#v", rows)
	}
	if len(snapshot.ActiveRequests) != 2 || snapshot.ActiveRequests[0] != "shared-model" || snapshot.ActiveRequests[1] != "shared-model" {
		t.Fatalf("duplicate active leases were not retained %#v", snapshot.ActiveRequests)
	}
	content, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), binaryDir) || strings.Contains(string(content), koboldPath) {
		t.Fatalf("node state exposed a configured path: %s", content)
	}
}

func TestGenerationCheckedUnloadDrainsLeasesAndRejectsStaleState(t *testing.T) {
	service, backend := newTestService(t, http.NotFoundHandler())
	runtime := service.textRuntime
	state := runtime.state
	state.mu.Lock()
	state.filename = "model.kcpps"
	state.modelID = "model"
	state.generation = 3
	state.users = 1
	state.leases = map[uint64]string{41: "model"}
	state.mu.Unlock()

	result := make(chan error, 1)
	go func() {
		result <- service.unloadLocalRuntime(context.Background(), siteapi.NodeUnloadRequest{
			NodeID: service.nodeID, BackendID: backendIDKoboldCPP, RuntimeID: runtime.name, ExpectedGeneration: 3,
		})
	}()
	waitForRuntimeUnloadWaiter(t, state)
	if backend.unloads.Load() != 0 {
		t.Fatal("backend unloaded before its active lease drained")
	}
	releaseActiveConfigLeaseOnce(state, 41)()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("generation-checked unload did not finish after lease release")
	}
	if backend.unloads.Load() != 1 {
		t.Fatalf("unexpected unload count %d", backend.unloads.Load())
	}
	if err := service.unloadRuntimeGeneration(context.Background(), runtime, 3); !errors.Is(err, errRuntimeGenerationChanged) {
		t.Fatalf("stale generation returned %v", err)
	}
	if backend.unloads.Load() != 1 {
		t.Fatal("stale generation reached the backend")
	}
}

func TestNodeStateClusterAuthenticationAndRemoteRouting(t *testing.T) {
	node, nodeBackend := newTestService(t, http.NotFoundHandler())
	node.nodeID = "worker"
	node.clusterRole = cluster.RoleSlave
	node.clusterToken = "secret"
	binaryPath := filepath.Join(t.TempDir(), "koboldcpp")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	node.backendBinaryPaths = map[string]string{backendIDKoboldCPP: binaryPath}
	node.textRuntime.state.mu.Lock()
	node.textRuntime.state.filename = "remote.kcpps"
	node.textRuntime.state.modelID = "remote-model"
	node.textRuntime.state.generation = 1
	node.textRuntime.state.mu.Unlock()

	unauthorized := httptest.NewRecorder()
	node.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/router/v1/node/state", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected node state status %d", unauthorized.Code)
	}

	nodeServer := httptest.NewServer(node)
	t.Cleanup(nodeServer.Close)
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "worker", NodeURL: nodeServer.URL}); err != nil {
		t.Fatal(err)
	}
	master, _ := newTestServiceWithRegistry(t, registry, http.NotFoundHandler(), "secret")
	master.nodeID = "master"
	master.clusterRole = cluster.RoleMaster

	stateRecorder := httptest.NewRecorder()
	master.ServeHTTP(stateRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/nodes/state?node_id=worker", nil))
	if stateRecorder.Code != http.StatusOK {
		t.Fatalf("remote state status=%d body=%s", stateRecorder.Code, stateRecorder.Body.String())
	}
	var snapshot siteapi.NodeState
	if err := json.NewDecoder(stateRecorder.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.NodeID != "worker" || len(snapshot.Backends) != 1 || snapshot.Backends[0].LoadedModels[0].ModelID != "remote-model" {
		t.Fatalf("unexpected remote snapshot %#v", snapshot)
	}

	body, err := json.Marshal(siteapi.NodeUnloadRequest{
		NodeID: "worker", BackendID: backendIDKoboldCPP, RuntimeID: node.textRuntime.name, ExpectedGeneration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	unloadRecorder := httptest.NewRecorder()
	master.ServeHTTP(unloadRecorder, httptest.NewRequest(http.MethodPost, "/router/v1/site/nodes/unload", bytes.NewReader(body)))
	if unloadRecorder.Code != http.StatusOK || nodeBackend.unloads.Load() != 1 {
		t.Fatalf("remote unload status=%d body=%s unloads=%d", unloadRecorder.Code, unloadRecorder.Body.String(), nodeBackend.unloads.Load())
	}

	missingRecorder := httptest.NewRecorder()
	master.ServeHTTP(missingRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/nodes/state?node_id=missing", nil))
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("unknown node status %d", missingRecorder.Code)
	}
}

func waitForRuntimeUnloadWaiter(t *testing.T, state *activeConfigState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.Lock()
		waiting := state.switchWaiters > 0
		state.mu.Unlock()
		if waiting {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("runtime unload never began waiting")
}
