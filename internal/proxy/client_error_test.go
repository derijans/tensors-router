package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
)

func TestRemoteDialFailureNamesTheNodeWithoutItsURL(t *testing.T) {
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion,
		NodeID:  "slave",
		NodeURL: unreachable.URL,
		Models:  []cluster.Model{testClusterModel("remote-only", "slave", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}
	service, _ := newTestServiceWithRegistry(t, registry, http.NotFoundHandler(), "secret")

	recorder := postChatCompletion(service, `{"model":"remote-only","messages":[]}`)

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadGateway || !strings.Contains(body, "node slave unavailable") || strings.Contains(body, "127.0.0.1") {
		t.Fatalf("remote dial failure status %d body %s", recorder.Code, body)
	}
}

func TestLocalBackendConnectionFailureHidesTheBackendURL(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		panic(http.ErrAbortHandler)
	}), "llm")

	recorder := postChatCompletion(service, `{"model":"llm","messages":[]}`)

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadGateway || !strings.Contains(body, "backend unavailable") || strings.Contains(body, "127.0.0.1") {
		t.Fatalf("local connection failure status %d body %s", recorder.Code, body)
	}
}

func postChatCompletion(service *Service, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	return recorder
}
