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

func routingGroupTextModel(modelID string, nodeID string, modelHash string, configHash string, source string, contextSize int) cluster.Model {
	model := testClusterModel(modelID, nodeID, modelHash, configHash, source)
	model.HasLLM = true
	model.Capabilities.Context = contextSize
	return model
}

func newTextRoutingGroupService(t *testing.T) *Service {
	t.Helper()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{
		routingGroupTextModel("llama-70b", "master", "weights", "config-a", cluster.SourceMaster, 8192),
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models: []cluster.Model{
			routingGroupTextModel("llama-70b-q8", "slave-a", "weights", "config-b", cluster.SourceSlave, 8192),
			routingGroupTextModel("mistral", "slave-a", "other-weights", "config-c", cluster.SourceSlave, 4096),
		},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	handle := routerstoretest.Open(t, routinggroups.SchemaModule{})
	service.routingGroups = routinggroups.NewStore(handle.DB(), handle.Reader())
	return service
}

func getTextRoutingGroups(t *testing.T, service *Service, query string) siteapi.TextRoutingGroupsResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/text-routing-groups"+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.TextRoutingGroupsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

// TestTextRoutingGroupsRequireSiteControl pins that a slave — which has no
// site control — cannot reach the text routing group endpoint at all, mirroring
// the image lane's admin-only surface.
func TestTextRoutingGroupsRequireSiteControl(t *testing.T) {
	service := newTextRoutingGroupService(t)
	service.clusterRole = cluster.RoleSlave
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/router/v1/site/text-routing-groups", nil)
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 for a slave with no site control", recorder.Code)
	}
}

func TestTextRoutingGroupCandidatesReportContextSize(t *testing.T) {
	service := newTextRoutingGroupService(t)
	response := getTextRoutingGroups(t, service, "?node_id=master&model_id=llama-70b")

	if len(response.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want both models on the other node", response.Candidates)
	}
	byModel := map[string]siteapi.TextRoutingGroupCandidate{}
	for _, candidate := range response.Candidates {
		byModel[candidate.ModelID] = candidate
	}
	if byModel["llama-70b-q8"].ContextSize != 8192 {
		t.Fatalf("context size = %d, want 8192", byModel["llama-70b-q8"].ContextSize)
	}
	if byModel["mistral"].ContextSize != 4096 {
		t.Fatalf("context size = %d, want 4096", byModel["mistral"].ContextSize)
	}
}

// TestTextRoutingGroupCandidatesExcludeIneligibleModels pins that a
// concurrent-serving model is still listed (so the UI can explain why), but
// marked ineligible rather than silently omitted or silently selectable.
func TestTextRoutingGroupCandidatesExcludeIneligibleModels(t *testing.T) {
	service := newTextRoutingGroupService(t)
	registry := service.registry
	concurrent := routingGroupTextModel("concurrent-model", "slave-a", "cweights", "cconfig", cluster.SourceSlave, 8192)
	concurrent.Options = map[string]json.RawMessage{"parallel": json.RawMessage("4")}
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models: []cluster.Model{
			routingGroupTextModel("llama-70b-q8", "slave-a", "weights", "config-b", cluster.SourceSlave, 8192),
			concurrent,
		},
	}); err != nil {
		t.Fatal(err)
	}

	response := getTextRoutingGroups(t, service, "?node_id=master&model_id=llama-70b")
	byModel := map[string]siteapi.TextRoutingGroupCandidate{}
	for _, candidate := range response.Candidates {
		byModel[candidate.ModelID] = candidate
	}
	candidate, found := byModel["concurrent-model"]
	if !found {
		t.Fatal("concurrent model was omitted rather than listed and marked ineligible")
	}
	if candidate.Eligible {
		t.Fatal("concurrent model was reported eligible")
	}
	if candidate.IneligibleReason == "" {
		t.Fatal("ineligible candidate carries no reason")
	}
}

func TestSaveTextRoutingGroupRejectsIneligibleMember(t *testing.T) {
	service := newTextRoutingGroupService(t)
	body := `{"anchor":{"node_id":"master","model_id":"llama-70b"},"members":[{"node_id":"slave-a","model_id":"mistral"}]}`

	registry := service.registry
	shortWindow := routingGroupTextModel("mistral", "slave-a", "other-weights", "config-c", cluster.SourceSlave, 0)
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models: []cluster.Model{
			routingGroupTextModel("llama-70b-q8", "slave-a", "weights", "config-b", cluster.SourceSlave, 8192),
			shortWindow,
		},
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/text-routing-groups", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s, want 400 for an ineligible member", recorder.Code, recorder.Body.String())
	}
}

// TestSaveTextRoutingGroupRebuildsTheRegistrySource pins that a saved group
// takes effect on the very next request, not at the next scheduling refresh
// tick.
func TestSaveTextRoutingGroupRebuildsTheRegistrySource(t *testing.T) {
	service := newTextRoutingGroupService(t)
	body := `{"anchor":{"node_id":"master","model_id":"llama-70b"},"members":[{"node_id":"slave-a","model_id":"llama-70b-q8"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/site/text-routing-groups", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}

	groupID, members, ok := service.registry.GroupMembers(cluster.GroupMember{Lane: cluster.RouteLaneText, NodeID: "master", ModelID: "llama-70b"})
	if !ok || groupID == "" || len(members) != 2 {
		t.Fatalf("registry group source was not rebuilt after save: groupID=%q members=%+v ok=%t", groupID, members, ok)
	}
}

func TestDeleteTextRoutingGroupClearsMembership(t *testing.T) {
	service := newTextRoutingGroupService(t)
	saveBody := `{"anchor":{"node_id":"master","model_id":"llama-70b"},"members":[{"node_id":"slave-a","model_id":"llama-70b-q8"}]}`
	saveRecorder := httptest.NewRecorder()
	saveRequest := httptest.NewRequest(http.MethodPost, "/router/v1/site/text-routing-groups", strings.NewReader(saveBody))
	saveRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(saveRecorder, saveRequest)
	if saveRecorder.Code != http.StatusOK {
		t.Fatalf("save status %d body %s", saveRecorder.Code, saveRecorder.Body.String())
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/router/v1/site/text-routing-groups?node_id=master&model_id=llama-70b", nil)
	service.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status %d body %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	if _, _, ok := service.registry.GroupMembers(cluster.GroupMember{Lane: cluster.RouteLaneText, NodeID: "master", ModelID: "llama-70b"}); ok {
		t.Fatal("group membership survived deletion")
	}
}
