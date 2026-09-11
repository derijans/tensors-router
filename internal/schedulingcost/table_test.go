package schedulingcost

import (
	"math"
	"testing"
)

func buildTestTable(t *testing.T) *Table {
	t.Helper()
	samples := []Sample{
		sampleFor("node-a", "sdxl", linearPoints(1000, 0.00002, imageWorks(40))),
		sampleFor("node-b", "sdxl-alt", linearPoints(600, 0.00001, imageWorks(40))),
		sampleFor("node-c", "sdxl-thin", linearPoints(500, 0.00003, imageWorks(5))),
	}
	loads := []LoadSample{
		{NodeID: "node-b", ConfigFilename: "sdxl-alt.kcpps", Count: 3, SumDuration: 57000},
	}
	return Build(samples, loads, nil, 20)
}

func sampleFor(nodeID string, modelID string, points [][2]float64) Sample {
	sample := sampleFromPoints(points)
	sample.NodeID = nodeID
	sample.ModelID = modelID
	return sample
}

func TestBuildKeepsOnlyQualifiedFits(t *testing.T) {
	table := buildTestTable(t)

	if _, ok := table.Estimate(ModelKey{NodeID: "node-a", ModelID: "sdxl", Section: "image"}); !ok {
		t.Fatal("qualified node was dropped from the table")
	}
	if _, ok := table.Estimate(ModelKey{NodeID: "node-c", ModelID: "sdxl-thin", Section: "image"}); ok {
		t.Fatal("node with only 5 samples was admitted under a floor of 20")
	}
}

func TestNilTableReportsUnqualified(t *testing.T) {
	var table *Table
	if _, ok := table.Estimate(ModelKey{NodeID: "node-a"}); ok {
		t.Fatal("nil table reported an estimate")
	}
	if _, ok := table.PredictMS(ModelKey{NodeID: "node-a"}, ImageWork(1)); ok {
		t.Fatal("nil table reported a prediction")
	}
	if _, ok := table.LoadMS(LoadKey{NodeID: "node-a"}); ok {
		t.Fatal("nil table reported a load cost")
	}
	if _, ok := table.TokenProfile(ProfileKey{NodeID: "node-a"}); ok {
		t.Fatal("nil table reported a token profile")
	}
}

// A queue pays the fixed per-request cost once per entry, so pricing it as a
// single request scaled by total work would badly understate a deep backlog.
func TestPredictQueueChargesBasePerEntry(t *testing.T) {
	table := buildTestTable(t)
	key := ModelKey{NodeID: "node-a", ModelID: "sdxl", Section: "image"}
	estimate, ok := table.Estimate(key)
	if !ok {
		t.Fatal("expected a qualified estimate")
	}

	const entries = 6
	const totalWork = 6 * 30 * 1024 * 1024
	queueMS, ok := table.PredictQueueMS(key, entries, ImageWork(totalWork))
	if !ok {
		t.Fatal("queue prediction was rejected for a qualified node")
	}
	want := entries*estimate.BaseMS + estimate.SlopeMS[0]*totalWork
	if math.Abs(queueMS-want) > 1e-6 {
		t.Fatalf("queue prediction = %v, want %v", queueMS, want)
	}
	single, ok := estimate.PredictMS(ImageWork(totalWork))
	if !ok {
		t.Fatal("single prediction rejected for matching arity")
	}
	if queueMS <= single {
		t.Fatalf("queue of %d priced at %v, no more than one request at %v", entries, queueMS, single)
	}
}

func TestPredictQueueOfNothingIsZero(t *testing.T) {
	table := buildTestTable(t)
	key := ModelKey{NodeID: "node-a", ModelID: "sdxl", Section: "image"}
	queueMS, ok := table.PredictQueueMS(key, 0, Work{})
	if !ok || queueMS != 0 {
		t.Fatalf("empty queue priced at %v ok=%t, want 0 true", queueMS, ok)
	}
}

// TestPredictRejectsWorkOfTheWrongArity pins the unit-safety guarantee added
// when prediction became work-vector-typed: a text-shaped work vector must
// never be priced against an image fit, or the reverse.
func TestPredictRejectsWorkOfTheWrongArity(t *testing.T) {
	table := buildTestTable(t)
	key := ModelKey{NodeID: "node-a", ModelID: "sdxl", Section: "image"}
	if _, ok := table.PredictMS(key, TextWork(100, 50)); ok {
		t.Fatal("image estimate accepted a two-term text work vector")
	}
	if _, ok := table.PredictQueueMS(key, 3, TextWork(100, 50)); ok {
		t.Fatal("image estimate accepted a two-term text work vector in a queue prediction")
	}
}

func TestMergeRestoresPublishedCosts(t *testing.T) {
	table := buildTestTable(t)
	key := ModelKey{NodeID: "node-a", ModelID: "sdxl", Section: "image"}
	original, _ := table.Estimate(key)

	merged := Merge(map[string]NodeCosts{
		"node-a": {Models: table.ModelCosts()},
		"node-b": {Loads: table.LoadCosts()},
	})

	restored, ok := merged.Estimate(key)
	if !ok {
		t.Fatal("merge dropped a published estimate")
	}
	if restored != original {
		t.Fatalf("restored %+v, want %+v", restored, original)
	}
	loadMS, ok := merged.LoadMS(LoadKey{NodeID: "node-b", ConfigFilename: "sdxl-alt.kcpps"})
	if !ok || loadMS != 19000 {
		t.Fatalf("restored load = %v ok=%t, want 19000 true", loadMS, ok)
	}
}

// TestMergeDropsCostsWithoutSlopes pins the deliberate degradation for a node
// on an older build: an empty SlopesMS (what slope_ms decodes as here) must be
// dropped as unqualified, never merged as a confident zero slope.
func TestMergeDropsCostsWithoutSlopes(t *testing.T) {
	merged := Merge(map[string]NodeCosts{
		"node-old": {Models: []ModelCost{{ModelID: "sdxl", Section: "image", BaseMS: 1000, Samples: 40}}},
	})
	if _, ok := merged.Estimate(ModelKey{NodeID: "node-old", ModelID: "sdxl", Section: "image"}); ok {
		t.Fatal("cost with no slopes was merged instead of dropped")
	}
}

// ModelCosts is published per node and merged back under the publishing node's
// id, so it must not carry a node id of its own that could disagree.
func TestPublishedCostsAreScopedByPublishingNode(t *testing.T) {
	table := buildTestTable(t)
	merged := Merge(map[string]NodeCosts{"node-z": {Models: table.ModelCosts()}})
	if _, ok := merged.Estimate(ModelKey{NodeID: "node-a", ModelID: "sdxl", Section: "image"}); ok {
		t.Fatal("costs kept their original node id instead of the publishing node")
	}
	if _, ok := merged.Estimate(ModelKey{NodeID: "node-z", ModelID: "sdxl", Section: "image"}); !ok {
		t.Fatal("costs were not attributed to the publishing node")
	}
}
