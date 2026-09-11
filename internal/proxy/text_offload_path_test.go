package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
)

const chatCompletionBody = `{"model":"llama-70b","messages":[{"role":"user","content":"hi"}]}`

// newGroupedTextService mirrors newGroupedImageService for the text lane: a
// node holding one LLM model, with a backend that blocks until the test
// releases it so queue states can be observed. The peer is declared as a
// group member but is deliberately not a routable replica, which is what
// lets these tests observe the queue directly rather than racing a second
// node.
func newGroupedTextService(t *testing.T, gate chan struct{}, grouped bool) *Service {
	t.Helper()
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
			return
		}
		if gate != nil {
			<-gate
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
	}), map[string]string{
		"llama-70b": `{"model_param":"C:\\models\\llm.gguf"}`,
	})
	service.clusterToken = "secret"

	registry := cluster.NewRegistry(cluster.RoleStandalone, service.nodeID, "http://local")
	local := testClusterModel("llama-70b", service.nodeID, "weights", "config-a", cluster.SourceLocal)
	local.Capabilities.Context = 8192
	if err := registry.UpdateLocal([]cluster.Model{local}); err != nil {
		t.Fatal(err)
	}
	if grouped {
		registry.SetGroupSource(newRoutingGroupLookup(nil, []routinggroups.TextGroup{{
			ID: "text-group",
			Members: []routinggroups.TextMember{
				{NodeID: service.nodeID, ModelID: "llama-70b"},
				{NodeID: "slave-a", ModelID: "llama-70b-alt"},
			},
		}}))
	}
	service.registry = registry
	return service
}

func postChat(service *Service) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(chatCompletionBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	return recorder
}

func postBorrowedChat(service *Service) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(chatCompletionBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, markBorrowedRequest(request))
	return recorder
}

func waitForTextBacklog(t *testing.T, service *Service, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, stats := range service.textQueue.Stats() {
			if stats.BacklogCount >= want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("text queue never reached a backlog of %d: %+v", want, service.textQueue.Stats())
}

// A grouped text model routes through the router-held queue exactly like a
// grouped image model does, and for the same reason: an admitted request is
// already inside the backend and cannot be moved.
func TestGroupedTextRequestQueuesBeforeTheBackend(t *testing.T) {
	gate := make(chan struct{})
	service := newGroupedTextService(t, gate, true)

	done := make(chan int, 3)
	for index := 0; index < 3; index++ {
		go func() { done <- postChat(service).Code }()
	}
	waitForTextBacklog(t, service, 3)

	stats := service.textQueue.Stats()
	if len(stats) != 1 || stats[0].GroupID != "text-group" {
		t.Fatalf("stats = %+v, want one group", stats)
	}

	close(gate)
	for index := 0; index < 3; index++ {
		if code := <-done; code != http.StatusOK {
			t.Fatalf("status %d, want 200", code)
		}
	}
}

// A model in no text group must not be queued at all, so nothing changes for
// traffic that was never enrolled — even though the service has a registry
// and a group source, unlike the no-registry-at-all case in
// ungrouped_text_test.go.
func TestUngroupedTextModelIsNeverQueued(t *testing.T) {
	service := newGroupedTextService(t, nil, false)

	if code := postChat(service).Code; code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if stats := service.textQueue.Stats(); len(stats) != 0 {
		t.Fatalf("stats = %+v, want nothing queued for an ungrouped model", stats)
	}
}

func TestBorrowedTextRequestIsReturnedWhenNativeWorkArrives(t *testing.T) {
	gate := make(chan struct{})
	service := newGroupedTextService(t, gate, true)

	native := make(chan int, 1)
	go func() { native <- postChat(service).Code }()
	waitForTextBacklog(t, service, 1)

	recorder := postBorrowedChat(service)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status %d body %s, want 409", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), offloadReturnedCode) {
		t.Fatalf("body %s, want the offload_returned code", recorder.Body.String())
	}

	close(gate)
	if code := <-native; code != http.StatusOK {
		t.Fatalf("native request status %d, want 200", code)
	}
}

func TestBorrowedTextRequestNeverForwardedRemotely(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleStandalone, "local-node", "http://local")
	remote := testClusterModel("remote-only", "slave-a", "weights", "config-a", cluster.SourceSlave)
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: "http://slave-a", Models: []cluster.Model{remote}}); err != nil {
		t.Fatal(err)
	}
	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("a borrowed request that resolves remotely must never reach any backend")
	}), "secret")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"remote-only","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, markBorrowedRequest(request))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status %d body %s, want 409", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), offloadReturnedCode) {
		t.Fatalf("body %s, want the offload_returned code", recorder.Body.String())
	}
}

func TestIdleNodeServesBorrowedTextWork(t *testing.T) {
	service := newGroupedTextService(t, nil, true)

	if recorder := postBorrowedChat(service); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s, want the borrowed request served", recorder.Code, recorder.Body.String())
	}
}

// newLeasedTextHelper registers a helper model on slave-a under its own,
// deliberately different, local id and stores a live lease naming it — the
// fixture every leasedHelperMember/leasedHelperContextFits test builds on.
func newLeasedTextHelper(t *testing.T, service *Service, helperContext int) {
	t.Helper()
	slave := testClusterModel("llama-70b-alt", "slave-a", "weights", "config-b", cluster.SourceSlave)
	slave.Capabilities.Context = helperContext
	if err := service.registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: "http://slave-a", Models: []cluster.Model{slave}}); err != nil {
		t.Fatal(err)
	}
	service.storeOffloadLease(offloadLease{
		Lane: cluster.RouteLaneText, GroupID: "text-group",
		OwnerNodeID: service.nodeID, HelperNodeID: "slave-a",
		ExpiresAt: time.Now().Add(time.Minute),
	})
}

// Routing groups link checkpoints that carry different ids on different
// nodes, so the helper's registry has never heard of the owner's id.
func TestLeasedHelperMemberResolvesTheHelpersOwnModelID(t *testing.T) {
	service := newGroupedTextService(t, nil, true)
	newLeasedTextHelper(t, service, 8192)

	helper, found := service.leasedHelperMember(cluster.RouteLaneText, "text-group", service.nodeID, "llama-70b")
	if !found {
		t.Fatal("no helper resolved for a live lease")
	}
	if helper.NodeID != "slave-a" || helper.ModelID != "llama-70b-alt" {
		t.Fatalf("helper = %+v, want slave-a/llama-70b-alt (the helper's own id, not the owner's)", helper)
	}
}

func TestOffloadedRequestBodyIsRewrittenToTheHelpersOwnModelID(t *testing.T) {
	rewritten := rewriteRequestModel([]byte(chatCompletionBody), "llama-70b-alt")
	var decoded struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(rewritten, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Model != "llama-70b-alt" {
		t.Fatalf("rewritten model = %q, want the helper's own id", decoded.Model)
	}
}

func TestLeasedHelperMemberRequiresALiveLease(t *testing.T) {
	service := newGroupedTextService(t, nil, true)
	if _, found := service.leasedHelperMember(cluster.RouteLaneText, "text-group", service.nodeID, "llama-70b"); found {
		t.Fatal("a helper was resolved with no active lease at all")
	}
}

// planOffloadLeases gates a lease on the owner's *average* pending request,
// so a single larger-than-average one can still be withdrawn toward a helper
// whose window cannot actually hold it — the gap leasedHelperContextFits closes.
func TestOffloadedTextRequestThatDoesNotFitTheHelperIsRequeued(t *testing.T) {
	service := newGroupedTextService(t, nil, true)
	newLeasedTextHelper(t, service, 100)
	helper, found := service.leasedHelperMember(cluster.RouteLaneText, "text-group", service.nodeID, "llama-70b")
	if !found {
		t.Fatal("no helper resolved for a live lease")
	}

	if service.leasedHelperContextFits(helper, 6000) {
		t.Fatal("a required context of 6000 was reported fitting a 100-token helper window")
	}
	if !service.leasedHelperContextFits(helper, 50) {
		t.Fatal("a required context of 50 was reported not fitting a 100-token helper window")
	}
	if !service.leasedHelperContextFits(helper, 0) {
		t.Fatal("an unsized entry (requiredContext <= 0) was gated instead of passed through")
	}
}

// The status a master polls has to describe the text half of the arrangement
// too: what this node still has to do, and whether it can take anything more.
func TestRuntimeStatusReportsTextQueueAndBorrowingState(t *testing.T) {
	gate := make(chan struct{})
	service := newGroupedTextService(t, gate, true)

	done := make(chan int, 1)
	go func() { done <- postChat(service).Code }()
	waitForTextBacklog(t, service, 1)

	status := service.localRuntimeStatus()
	if status.AcceptingBorrowedText {
		t.Fatal("node advertises it is accepting borrowed text work while running its own")
	}
	if len(status.TextQueue) != 1 || status.TextQueue[0].GroupID != "text-group" {
		t.Fatalf("text queue = %+v, want the group backlog", status.TextQueue)
	}

	close(gate)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	waitForCondition(t, func() bool { return service.localRuntimeStatus().AcceptingBorrowedText })
}
