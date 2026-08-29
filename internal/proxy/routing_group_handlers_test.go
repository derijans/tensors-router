package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/siteapi"
)

func routingGroupImageModel(imageID string, nodeID string, modelHash string, configHash string, source string) cluster.Model {
	model := testClusterModel(imageID, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasImage = true
	model.ImageID = imageID
	model.PublicImageID = imageID
	model.BackendMode = cluster.BackendModeLlamaSDCPP
	return model
}

// The fixture is the case the feature exists for: one checkpoint, configured
// differently on two nodes, plus an unrelated checkpoint on a third.
func newRoutingGroupService(t *testing.T) *Service {
	t.Helper()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{
		routingGroupImageModel("sdxl", "master", "weights", "config-a", cluster.SourceMaster),
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models: []cluster.Model{
			routingGroupImageModel("xl-jugg-q8", "slave-a", "weights", "config-b", cluster.SourceSlave),
			routingGroupImageModel("flux", "slave-a", "other-weights", "config-c", cluster.SourceSlave),
		},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	store, err := routinggroups.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	})
	service.routingGroups = store
	return service
}

func getRoutingGroups(t *testing.T, service *Service, query string) siteapi.RoutingGroupsResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/routing-groups"+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.RoutingGroupsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

// Filtering candidates by name or config hash would hide exactly the models worth
// grouping, so the list is deliberately unfiltered and labelled instead.
func TestRoutingGroupCandidatesIncludeDifferentNamesAndConfigs(t *testing.T) {
	service := newRoutingGroupService(t)
	response := getRoutingGroups(t, service, "?node_id=master&image_id=sdxl")

	if len(response.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want both models on the other node", response.Candidates)
	}
	byImage := map[string]siteapi.RoutingGroupCandidate{}
	for _, candidate := range response.Candidates {
		byImage[candidate.ImageID] = candidate
	}
	sameWeights, ok := byImage["xl-jugg-q8"]
	if !ok {
		t.Fatal("the same checkpoint under another config was not offered")
	}
	if sameWeights.ConfigHash == "config-a" {
		t.Fatal("candidate should differ from the anchor by config hash")
	}
	if !sameWeights.WeightsMatch {
		t.Fatal("matching model hash was not reported as the same weights")
	}
	if different, ok := byImage["flux"]; !ok || different.WeightsMatch {
		t.Fatalf("a different checkpoint was reported as matching weights: %+v", different)
	}
}

func TestRoutingGroupCandidatesExcludeTheAnchorNode(t *testing.T) {
	service := newRoutingGroupService(t)
	response := getRoutingGroups(t, service, "?node_id=master&image_id=sdxl")
	for _, candidate := range response.Candidates {
		if candidate.NodeID == "master" {
			t.Fatalf("candidate %+v is on the anchor node, which cannot help itself", candidate)
		}
	}
}

func TestRoutingGroupSaveAndReadBack(t *testing.T) {
	service := newRoutingGroupService(t)

	body := `{"anchor":{"node_id":"master","image_id":"sdxl"},"members":[{"node_id":"slave-a","image_id":"xl-jugg-q8"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/routing-groups", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	response := getRoutingGroups(t, service, "?node_id=master&image_id=sdxl")
	if len(response.Groups) != 1 || len(response.Groups[0].Members) != 2 {
		t.Fatalf("groups = %+v, want one group of two", response.Groups)
	}
	selected := 0
	for _, candidate := range response.Candidates {
		if candidate.Selected {
			selected++
			if candidate.ImageID != "xl-jugg-q8" {
				t.Fatalf("wrong candidate marked selected: %+v", candidate)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("selected candidates = %d, want 1", selected)
	}
}

// A saved group has to be visible to routing straight away, not at the next
// refresh tick, or the operator sees no effect from what they just did.
func TestSavingAGroupUpdatesRoutingImmediately(t *testing.T) {
	service := newRoutingGroupService(t)
	body := `{"anchor":{"node_id":"master","image_id":"sdxl"},"members":[{"node_id":"slave-a","image_id":"xl-jugg-q8"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/routing-groups", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	// Asserted through routing rather than by reading the registry: with the peer
	// priced as the faster option, a request for the anchor can only reach it if
	// the group is live.
	service.registry.SetCostSource(&reachableCostSource{fastNodeID: "slave-a"})
	route, release, ok := service.registry.AcquireImage("sdxl", true, "*", cluster.RouteHint{Work: 1000})
	if !ok {
		t.Fatal("no route after saving the group")
	}
	defer release()
	if route.NodeID != "slave-a" || route.LocalImageID != "xl-jugg-q8" {
		t.Fatalf("route = %+v, want the grouped peer on the other node", route)
	}
	if route.PublicImageID != "sdxl" {
		t.Fatalf("public image id = %q, want the requested id", route.PublicImageID)
	}
}

// reachableCostSource makes one node clearly cheapest and every node qualified, so
// a test can assert which member routing actually reaches.
type reachableCostSource struct {
	fastNodeID string
}

func (source *reachableCostSource) perJobMS(nodeID string) float64 {
	if nodeID == source.fastNodeID {
		return 1000
	}
	return 60000
}

func (source *reachableCostSource) PredictMS(nodeID string, modelID string, lane string, work float64) (float64, bool) {
	return source.perJobMS(nodeID), true
}

func (source *reachableCostSource) PredictQueueMS(nodeID string, modelID string, lane string, count int64, work float64) (float64, bool) {
	return float64(count) * source.perJobMS(nodeID), true
}

func (source *reachableCostSource) SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool) {
	return 0, true
}

func (source *reachableCostSource) NodeBacklog(nodeID string, groupID string) (int64, float64) {
	return 0, 0
}

func TestRoutingGroupDelete(t *testing.T) {
	service := newRoutingGroupService(t)
	body := `{"anchor":{"node_id":"master","image_id":"sdxl"},"members":[{"node_id":"slave-a","image_id":"xl-jugg-q8"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/routing-groups", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	recorder = httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/router/v1/site/routing-groups?node_id=master&image_id=sdxl", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	if response := getRoutingGroups(t, service, ""); len(response.Groups) != 0 {
		t.Fatalf("groups = %+v, want none after delete", response.Groups)
	}
}

func TestRoutingGroupDeleteRequiresAnAnchor(t *testing.T) {
	service := newRoutingGroupService(t)
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/router/v1/site/routing-groups", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 without an anchor", recorder.Code)
	}
}

// Site control endpoints are admin surface and must stay closed on a slave.
func TestRoutingGroupEndpointIsClosedOnASlave(t *testing.T) {
	service := newRoutingGroupService(t)
	service.clusterRole = cluster.RoleSlave

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/routing-groups", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 on a slave", recorder.Code)
	}
}
