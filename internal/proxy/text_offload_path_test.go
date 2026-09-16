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

func newLinkedTextService(t *testing.T, gate chan struct{}, linked bool) *Service {
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
	if linked {
		service.installRoutingLinks(routingLinkSnapshot{Text: []routinggroups.Link{{
			Owner:  routinggroups.Endpoint{NodeID: service.nodeID, ModelID: "llama-70b"},
			Helper: routinggroups.Endpoint{NodeID: "slave-a", ModelID: "llama-70b-alt"},
		}}})
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
	service.ServeHTTP(recorder, markBorrowLoadAllowed(markBorrowedRequest(request)))
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

func TestLinkedTextRequestQueuesBeforeTheBackend(t *testing.T) {
	gate := make(chan struct{})
	service := newLinkedTextService(t, gate, true)

	done := make(chan int, 3)
	for index := 0; index < 3; index++ {
		go func() { done <- postChat(service).Code }()
	}
	waitForTextBacklog(t, service, 3)

	stats := service.textQueue.Stats()
	if len(stats) != 1 || stats[0].ModelID != "llama-70b" {
		t.Fatalf("stats = %+v, want the backlog reported under the model", stats)
	}

	close(gate)
	for index := 0; index < 3; index++ {
		if code := <-done; code != http.StatusOK {
			t.Fatalf("status %d, want 200", code)
		}
	}
}

func TestUnlinkedTextModelIsNeverQueued(t *testing.T) {
	service := newLinkedTextService(t, nil, false)

	if code := postChat(service).Code; code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if stats := service.textQueue.Stats(); len(stats) != 0 {
		t.Fatalf("stats = %+v, want nothing queued for an unlinked model", stats)
	}
}

func TestBorrowedTextRequestIsReturnedWhenNativeWorkArrives(t *testing.T) {
	gate := make(chan struct{})
	service := newLinkedTextService(t, gate, true)

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
	service := newLinkedTextService(t, nil, true)

	if recorder := postBorrowedChat(service); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s, want the borrowed request served", recorder.Code, recorder.Body.String())
	}
}

func TestBorrowedTextWorkIsReturnedWhenServingItWouldLoadAForbiddenModel(t *testing.T) {
	service := newLinkedTextService(t, nil, true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(chatCompletionBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, markBorrowedRequest(request))

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), offloadReturnedCode) {
		t.Fatalf("status %d body %s, want 409 offload_returned when the link forbids loading", recorder.Code, recorder.Body.String())
	}
	if filename := service.activeTextConfigFilename(); filename != "" {
		t.Fatalf("active text config = %q, want nothing loaded for the refused request", filename)
	}
}

func TestRelayAddressesTextWorkToTheHelpersOwnModel(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/router/v1/node/offload/request", strings.NewReader(chatCompletionBody))
	lease := offloadLease{Lane: cluster.RouteLaneText, OwnerModelID: "llama-70b", HelperNodeID: "slave-a", HelperModelID: "llama-70b-alt"}

	_, body := requestAddressedToHelper(request, []byte(chatCompletionBody), lease)

	var decoded struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Model != "llama-70b-alt" {
		t.Fatalf("relayed model = %q, want the helper's own id", decoded.Model)
	}
}

func TestLeasedHelperContextFitsUsesTheHelpersWindow(t *testing.T) {
	service := newLinkedTextService(t, nil, true)
	slave := testClusterModel("llama-70b-alt", "slave-a", "weights", "config-b", cluster.SourceSlave)
	slave.Capabilities.Context = 100
	if err := service.registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: "slave-a", NodeURL: "http://slave-a", Models: []cluster.Model{slave}}); err != nil {
		t.Fatal(err)
	}
	helper := routinggroups.Endpoint{NodeID: "slave-a", ModelID: "llama-70b-alt"}

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

func TestRuntimeStatusReportsTextQueueAndBorrowingState(t *testing.T) {
	gate := make(chan struct{})
	service := newLinkedTextService(t, gate, true)

	done := make(chan int, 1)
	go func() { done <- postChat(service).Code }()
	waitForTextBacklog(t, service, 1)

	status := service.localRuntimeStatus()
	if status.AcceptingBorrowedText {
		t.Fatal("node advertises it is accepting borrowed text work while running its own")
	}
	if len(status.TextQueue) != 1 || status.TextQueue[0].ModelID != "llama-70b" {
		t.Fatalf("text queue = %+v, want the model backlog", status.TextQueue)
	}

	close(gate)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	waitForCondition(t, func() bool { return service.localRuntimeStatus().AcceptingBorrowedText })
}
