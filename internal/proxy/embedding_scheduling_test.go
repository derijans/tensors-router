package proxy

import (
	"context"
	"io"
	"log"
	"testing"

	"tensors-router/internal/cluster"
)

func TestSelectorlessEmbeddingCandidatesDeduplicateAliasesAndFilterUnavailableModels(t *testing.T) {
	primary := cluster.Model{
		PublicID: "embed", LocalID: "embed", NodeID: "node-a", Filename: "embed.kcpps",
		BackendMode: BackendModeKobold, HasEmbeddings: true, Available: true, EmbeddingsLoaded: true,
	}
	alias := primary
	alias.PublicID = "embed-alias"
	unavailable := primary
	unavailable.NodeID = "node-b"
	unavailable.Available = false
	unloaded := primary
	unloaded.NodeID = "node-c"
	unloaded.EmbeddingsLoaded = false
	nonEmbedding := primary
	nonEmbedding.NodeID = "node-d"
	nonEmbedding.HasEmbeddings = false

	candidates := selectorlessRegistryEmbeddingCandidates([]cluster.Model{alias, unavailable, unloaded, nonEmbedding, primary}, "/v1/embeddings")
	if len(candidates) != 1 || candidates[0].PublicID != "embed" {
		t.Fatalf("unexpected candidates %#v", candidates)
	}
}

func TestSelectorlessEmbeddingAcquisitionSkipsStaleCandidate(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	available := cluster.Model{
		PublicID: "embed-b", LocalID: "embed-b", NodeID: "node-b", NodeURL: "http://node-b", Filename: "embed-b.kcpps",
		BackendMode: BackendModeKobold, Source: cluster.SourceSlave, HasEmbeddings: true, Available: true, EmbeddingsLoaded: true,
	}
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: available.NodeID, NodeURL: available.NodeURL, Models: []cluster.Model{available}}); err != nil {
		t.Fatal(err)
	}
	stale := available
	stale.PublicID = "embed-a"
	stale.LocalID = "embed-a"
	stale.NodeID = "node-a"
	stale.NodeURL = "http://node-a"
	stale.Filename = "embed-a.kcpps"
	service := NewService(ServiceConfig{Registry: registry, Logger: log.New(io.Discard, "", 0)})

	target, ok := service.acquireSelectorlessRegistryEmbeddingCandidates("/v1/embeddings", context.Background(), []cluster.Model{stale, available})
	if !ok || target.publicID != "embed-b" {
		t.Fatalf("available candidate was not used target=%#v ok=%t", target, ok)
	}
	target.release()
}

func TestSelectorlessEmbeddingCandidatesCoverEmbeddingPaths(t *testing.T) {
	model := cluster.Model{
		PublicID: "embed", LocalID: "embed", NodeID: "node-a", Filename: "embed.kcpps",
		BackendMode: BackendModeKobold, HasEmbeddings: true, Available: true, EmbeddingsLoaded: true,
	}
	for _, path := range []string{"/v1/embeddings", "/v2/embed", "/api/embed", "/api/extra/embeddings", "/rerank", "/v1/rerank", "/v2/rerank", "/classify", "/score", "/v1/score", "/pooling"} {
		t.Run(path, func(t *testing.T) {
			if candidates := selectorlessRegistryEmbeddingCandidates([]cluster.Model{model}, path); len(candidates) != 1 {
				t.Fatalf("path %s rejected loaded embedding model", path)
			}
		})
	}
}
