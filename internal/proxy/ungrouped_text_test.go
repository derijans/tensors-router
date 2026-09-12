package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
)

func TestUngroupedTextRequestIsNeverQueued(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
		}),
		http.NotFoundHandler(),
		map[string]string{
			"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"text-only","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if service.textQueue != nil {
		if stats := service.textQueue.Stats(); len(stats) != 0 {
			t.Fatalf("stats = %+v, want nothing queued for an ungrouped model with no registry at all", stats)
		}
	}
}

func TestUngroupedTextRequestUsesPlainRotation(t *testing.T) {
	service := newTextRoutingGroupService(t)

	groupID, members, ok := service.registry.GroupMembers(cluster.GroupMember{Lane: cluster.RouteLaneText, NodeID: "master", ModelID: "llama-70b"})
	if ok || groupID != "" || members != nil {
		t.Fatalf("an ungrouped model reported group membership: id=%q members=%+v ok=%t", groupID, members, ok)
	}

	route, release, ok := service.registry.Acquire("llama-70b", true, cluster.RouteHint{})
	if !ok {
		t.Fatal("no route for an ungrouped local model")
	}
	defer release()
	if route.Remote || route.NodeID != "master" {
		t.Fatalf("route = %+v, want the untouched local-first cascade", route)
	}
}
