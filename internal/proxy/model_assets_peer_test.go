package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/modelassets"
)

func newAssetIndexForTest(t *testing.T) *modelassets.Index {
	t.Helper()
	root := t.TempDir()
	index, err := modelassets.NewIndex(filepath.Join(root, "store"), filepath.Join(root, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	return index
}

func indexAssetForTest(t *testing.T, index *modelassets.Index, filename string, content []byte) modelassets.Asset {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	asset, err := index.IndexFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func startMasterWithAsset(t *testing.T, masterPublicURL string, filename string, content []byte) (*httptest.Server, modelassets.Asset) {
	t.Helper()
	masterIndex := newAssetIndexForTest(t)
	asset := indexAssetForTest(t, masterIndex, filename, content)
	master := NewService(ServiceConfig{
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		NodeURL:      masterPublicURL,
		ClusterToken: "secret",
		AssetIndex:   masterIndex,
	})
	server := httptest.NewServer(master)
	t.Cleanup(server.Close)
	return server, asset
}

func newSlaveService(t *testing.T, masterURL string) *Service {
	t.Helper()
	return NewService(ServiceConfig{
		ClusterRole:   cluster.RoleSlave,
		NodeID:        "slave-1",
		NodeURL:       "http://slave.invalid:18081",
		MasterURL:     masterURL,
		ClusterToken:  "secret",
		ClusterClient: cluster.NewClient("secret"),
		AssetIndex:    newAssetIndexForTest(t),
	})
}

func assertSlavePulledAsset(t *testing.T, slave *Service, asset modelassets.Asset, content []byte) {
	t.Helper()
	path, found := slave.resolvePeerAssetPath(asset.SHA256, asset.Filename)
	if !found {
		t.Fatalf("slave did not resolve the master asset %s", asset.SHA256)
	}
	pulled, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pulled, content) {
		t.Fatalf("pulled content %q does not match the master copy %q", pulled, content)
	}
}

func TestSlaveResolvesMasterAssetWhenMasterPublicURLIsUnset(t *testing.T) {
	content := []byte("master weights without a public url")
	server, asset := startMasterWithAsset(t, "", "master-only.gguf", content)
	slave := newSlaveService(t, server.URL)

	sources := slave.lookupCoordinatedAssetSources(asset.SHA256)
	if len(sources) != 1 || sources[0].NodeURL != server.URL {
		t.Fatalf("slave did not point the master record at its configured master url: %#v", sources)
	}
	assertSlavePulledAsset(t, slave, asset, content)
}

func TestSlaveResolvesMasterAssetWhenMasterAdvertisesAnotherURL(t *testing.T) {
	const advertisedURL = "http://master.example:18080"
	content := []byte("master weights behind another name")
	server, asset := startMasterWithAsset(t, advertisedURL, "master-renamed.gguf", content)
	slave := newSlaveService(t, server.URL)

	var advertised assetLookupResponse
	if err := slave.clusterClient.JSON(t.Context(), http.MethodPost, server.URL, "/router/v1/node/assets/lookup-cluster", assetLookupRequest{Hashes: []string{asset.SHA256}}, &advertised); err != nil {
		t.Fatal(err)
	}
	if len(advertised.Assets) != 1 || advertised.Assets[0].NodeURL != advertisedURL {
		t.Fatalf("master did not advertise the unreachable url this test covers: %#v", advertised.Assets)
	}

	sources := slave.lookupCoordinatedAssetSources(asset.SHA256)
	if len(sources) != 1 || sources[0].NodeURL != server.URL {
		t.Fatalf("slave did not replace the unreachable master url: %#v", sources)
	}
	assertSlavePulledAsset(t, slave, asset, content)
}

func TestSlaveKeepsPeerAssetSourcesFromClusterLookup(t *testing.T) {
	const peerURL = "http://peer.invalid:18082"
	hash := strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(assetLookupResponse{Assets: []assetLookupRecord{
			{SHA256: hash, Filename: "weights.gguf", Size: 7, NodeURL: "http://master.example:18080", Master: true},
			{SHA256: hash, Filename: "weights.gguf", Size: 7, NodeURL: peerURL},
		}})
	}))
	defer server.Close()
	slave := newSlaveService(t, server.URL)

	sources := slave.lookupCoordinatedAssetSources(hash)
	if len(sources) != 2 {
		t.Fatalf("unexpected sources: %#v", sources)
	}
	if sources[0].NodeURL != server.URL || sources[0].Master {
		t.Fatalf("master record was not repointed at the configured master url: %#v", sources[0])
	}
	if sources[1].NodeURL != peerURL {
		t.Fatalf("peer record url was rewritten: %#v", sources[1])
	}
}

func TestClusterAssetLookupMarksMasterOwnedRecords(t *testing.T) {
	const advertisedURL = "http://master.example:18080"
	masterIndex := newAssetIndexForTest(t)
	asset := indexAssetForTest(t, masterIndex, "owned.gguf", []byte("owned weights"))
	master := NewService(ServiceConfig{
		ClusterRole:  cluster.RoleMaster,
		NodeID:       "master",
		NodeURL:      advertisedURL,
		ClusterToken: "secret",
		AssetIndex:   masterIndex,
	})

	response := master.lookupClusterAssets(t.Context(), assetLookupRequest{Hashes: []string{asset.SHA256}})
	if len(response.Assets) != 1 {
		t.Fatalf("unexpected cluster lookup response: %#v", response.Assets)
	}
	if !response.Assets[0].Master || response.Assets[0].NodeURL != advertisedURL {
		t.Fatalf("master record is not marked as master owned: %#v", response.Assets[0])
	}
	if path, found := master.resolvePeerAssetPath(asset.SHA256, asset.Filename); found {
		t.Fatalf("master transferred its own asset from itself: %s", path)
	}
}

func TestAssetLookupRecordWireFormatStaysCompatible(t *testing.T) {
	var legacy assetLookupRecord
	if err := json.Unmarshal([]byte(`{"sha256":"abc","filename":"weights.gguf","size":7,"node_url":"http://peer.invalid:18082"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Master {
		t.Fatalf("legacy record was decoded as master owned: %#v", legacy)
	}
	encoded, err := json.Marshal(assetLookupRecord{SHA256: "abc", Filename: "weights.gguf", Size: 7, NodeURL: "http://peer.invalid:18082"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("master")) {
		t.Fatalf("peer record wire format changed: %s", encoded)
	}
}
