package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routerstore/routerstoretest"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/siteapi"
)

const (
	imageLinksPath = "/router/v1/site/routing-groups"
	textLinksPath  = "/router/v1/site/text-routing-groups"
)

func routingImageModel(imageID string, nodeID string, modelHash string, configHash string, source string) cluster.Model {
	model := testClusterModel(imageID, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasImage = true
	model.ImageID = imageID
	model.PublicImageID = imageID
	model.BackendMode = cluster.BackendModeLlamaSDCPP
	return model
}

func routingTextModel(modelID string, nodeID string, modelHash string, configHash string, source string, contextSize int) cluster.Model {
	model := testClusterModel(modelID, nodeID, modelHash, configHash, source)
	model.HasLLM = true
	model.Capabilities.Context = contextSize
	return model
}

func newRoutingLinkService(t *testing.T, local []cluster.Model, slaveModels []cluster.Model) *Service {
	t.Helper()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal(local); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: "http://slave-a", Models: slaveModels}); err != nil {
		t.Fatal(err)
	}
	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	handle := routerstoretest.Open(t, routinggroups.SchemaModule{})
	service.routingGroups = routinggroups.NewStore(handle.DB(), handle.Reader())
	return service
}

// The fixture is the case the feature exists for: one checkpoint, configured
// differently on two nodes, plus an unrelated checkpoint on the second node.
func newImageRoutingLinkService(t *testing.T) *Service {
	return newRoutingLinkService(t,
		[]cluster.Model{routingImageModel("cc-ff", "master", "ff", "config-cc", cluster.SourceMaster)},
		[]cluster.Model{
			routingImageModel("cc11-ff", "slave-a", "ff", "config-cc11", cluster.SourceSlave),
			routingImageModel("flux", "slave-a", "other-weights", "config-flux", cluster.SourceSlave),
		})
}

func newTextRoutingLinkService(t *testing.T) *Service {
	return newRoutingLinkService(t,
		[]cluster.Model{routingTextModel("llama-70b", "master", "weights", "config-a", cluster.SourceMaster, 8192)},
		[]cluster.Model{
			routingTextModel("llama-70b-q8", "slave-a", "weights", "config-b", cluster.SourceSlave, 8192),
			routingTextModel("mistral", "slave-a", "other-weights", "config-c", cluster.SourceSlave, 4096),
		})
}

func getRoutingLinks(t *testing.T, service *Service, path string, query string) siteapi.RoutingLinksResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.RoutingLinksResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func postRoutingLinks(service *Service, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	return recorder
}

func candidatesByModel(candidates []siteapi.RoutingCandidate) map[string]siteapi.RoutingCandidate {
	byModel := map[string]siteapi.RoutingCandidate{}
	for _, candidate := range candidates {
		byModel[candidate.ModelID] = candidate
	}
	return byModel
}

// Filtering candidates by name or config hash would hide exactly the models worth
// linking, so the list is deliberately unfiltered and labelled instead.
func TestRoutingCandidatesIncludeDifferentNamesAndConfigsButNotTheAnchorNode(t *testing.T) {
	service := newImageRoutingLinkService(t)
	response := getRoutingLinks(t, service, imageLinksPath, "?node_id=master&model_id=cc-ff")

	byModel := candidatesByModel(response.Candidates)
	if len(byModel) != 2 {
		t.Fatalf("candidates = %+v, want both models on the other node only", response.Candidates)
	}
	if sameWeights, ok := byModel["cc11-ff"]; !ok || !sameWeights.WeightsMatch {
		t.Fatalf("cc11-ff = %+v, want it offered as the same weights", sameWeights)
	}
	if different, ok := byModel["flux"]; !ok || different.WeightsMatch {
		t.Fatalf("flux = %+v, want it offered as different weights", different)
	}
}

func TestSavedLinksKeepTheirDirectionAndFlags(t *testing.T) {
	service := newImageRoutingLinkService(t)
	body := `{"anchor":{"node_id":"slave-a","model_id":"cc11-ff"},` +
		`"lends_to":[],` +
		`"borrows_from":[{"node_id":"master","model_id":"cc-ff","load_if_unloaded":false,"restore_after_borrow":true}]}`
	if recorder := postRoutingLinks(service, imageLinksPath, body); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	fromSlave := getRoutingLinks(t, service, imageLinksPath, "?node_id=slave-a&model_id=cc11-ff")
	master := candidatesByModel(fromSlave.Candidates)["cc-ff"]
	if master.LendsTo.Selected {
		t.Fatalf("cc-ff = %+v, want cc11-ff not lending to the master", master)
	}
	wantBorrow := siteapi.RoutingLinkState{Selected: true, LoadIfUnloaded: false, RestoreAfterBorrow: true}
	if master.BorrowsFrom != wantBorrow {
		t.Fatalf("borrows_from cc-ff = %+v, want %+v", master.BorrowsFrom, wantBorrow)
	}

	fromMaster := getRoutingLinks(t, service, imageLinksPath, "?node_id=master&model_id=cc-ff")
	slave := candidatesByModel(fromMaster.Candidates)["cc11-ff"]
	if !slave.LendsTo.Selected || slave.BorrowsFrom.Selected {
		t.Fatalf("cc11-ff seen from the master = %+v, want only the master lending to it", slave)
	}
}

// A saved link has to reach request handling straight away, not at the next
// refresh tick, or the operator sees no effect from what they just did.
func TestSavingLinksMakesTheModelsQueueImmediately(t *testing.T) {
	service := newImageRoutingLinkService(t)
	body := `{"anchor":{"node_id":"master","model_id":"cc-ff"},"lends_to":[{"node_id":"slave-a","model_id":"cc11-ff","load_if_unloaded":true}],"borrows_from":[]}`
	if recorder := postRoutingLinks(service, imageLinksPath, body); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	if !service.scheduler.queuesForLending(cluster.RouteLaneImage, "master", "cc-ff") {
		t.Fatal("the owner does not queue for lending after saving its link")
	}
	if service.scheduler.queuesForLending(cluster.RouteLaneImage, "slave-a", "flux") {
		t.Fatal("an unlinked model queues for lending")
	}
}

func TestDeletingAnAnchorRemovesLinksInBothDirections(t *testing.T) {
	service := newImageRoutingLinkService(t)
	body := `{"anchor":{"node_id":"master","model_id":"cc-ff"},` +
		`"lends_to":[{"node_id":"slave-a","model_id":"cc11-ff"}],` +
		`"borrows_from":[{"node_id":"slave-a","model_id":"flux"}]}`
	if recorder := postRoutingLinks(service, imageLinksPath, body); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, imageLinksPath+"?node_id=master&model_id=cc-ff", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if response := getRoutingLinks(t, service, imageLinksPath, ""); len(response.Links) != 0 {
		t.Fatalf("links = %+v, want none after delete", response.Links)
	}
	if service.scheduler.queuesForLending(cluster.RouteLaneImage, "master", "cc-ff") {
		t.Fatal("the deleted anchor still queues for lending")
	}
}

func TestDeletingLinksRequiresAnAnchor(t *testing.T) {
	service := newImageRoutingLinkService(t)
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, imageLinksPath, nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 without an anchor", recorder.Code)
	}
}

// Site control endpoints are admin surface and must stay closed on a slave.
func TestRoutingLinkEndpointsAreClosedOnASlave(t *testing.T) {
	service := newImageRoutingLinkService(t)
	service.clusterRole = cluster.RoleSlave

	for _, path := range []string{imageLinksPath, textLinksPath} {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status %d, want 404 on a slave", path, recorder.Code)
		}
	}
}

func TestTextCandidatesReportContextSizeAndEligibility(t *testing.T) {
	service := newTextRoutingLinkService(t)
	concurrent := routingTextModel("concurrent-model", "slave-a", "cweights", "cconfig", cluster.SourceSlave, 8192)
	concurrent.Options = map[string]json.RawMessage{"parallel": json.RawMessage("4")}
	if err := service.registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: "http://slave-a", Models: []cluster.Model{
		routingTextModel("llama-70b-q8", "slave-a", "weights", "config-b", cluster.SourceSlave, 8192),
		concurrent,
	}}); err != nil {
		t.Fatal(err)
	}

	byModel := candidatesByModel(getRoutingLinks(t, service, textLinksPath, "?node_id=master&model_id=llama-70b").Candidates)
	if byModel["llama-70b-q8"].ContextSize != 8192 || !byModel["llama-70b-q8"].Eligible {
		t.Fatalf("llama-70b-q8 = %+v, want an eligible candidate reporting 8192", byModel["llama-70b-q8"])
	}
	candidate, found := byModel["concurrent-model"]
	if !found || candidate.Eligible || candidate.IneligibleReason == "" {
		t.Fatalf("concurrent-model = %+v (found=%t), want it listed as ineligible with a reason", candidate, found)
	}
}

func TestSavingTextLinksRejectsAnIneligiblePeerInEitherDirection(t *testing.T) {
	for _, direction := range []string{"lends_to", "borrows_from"} {
		service := newTextRoutingLinkService(t)
		noWindow := routingTextModel("mistral", "slave-a", "other-weights", "config-c", cluster.SourceSlave, 0)
		if err := service.registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: "http://slave-a", Models: []cluster.Model{noWindow}}); err != nil {
			t.Fatal(err)
		}
		body := `{"anchor":{"node_id":"master","model_id":"llama-70b"},"` + direction + `":[{"node_id":"slave-a","model_id":"mistral"}]}`
		if recorder := postRoutingLinks(service, textLinksPath, body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d body %s, want 400 for an ineligible peer", direction, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTextLinksAreStoredApartFromImageLinks(t *testing.T) {
	service := newTextRoutingLinkService(t)
	body := `{"anchor":{"node_id":"master","model_id":"llama-70b"},"lends_to":[{"node_id":"slave-a","model_id":"llama-70b-q8","load_if_unloaded":true}]}`
	if recorder := postRoutingLinks(service, textLinksPath, body); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	if links := getRoutingLinks(t, service, textLinksPath, "").Links; len(links) != 1 {
		t.Fatalf("text links = %+v, want one", links)
	}
	if links := getRoutingLinks(t, service, imageLinksPath, "").Links; len(links) != 0 {
		t.Fatalf("image links = %+v, want none", links)
	}
	if !service.scheduler.queuesForLending(cluster.RouteLaneText, "master", "llama-70b") {
		t.Fatal("the text owner does not queue for lending after saving its link")
	}
}
