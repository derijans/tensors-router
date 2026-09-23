package proxy

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
)

func TestOffloadRelayStreamsEachHelperEventWithoutWaitingForTheEnd(t *testing.T) {
	releaseHelper := make(chan struct{})
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		<-releaseHelper
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(helper.Close)

	master, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	master.clusterToken = "secret"
	master.clusterClient = cluster.NewClient("secret")
	if err := master.clusterClient.AllowBaseURLs(helper.URL); err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, master.nodeID, "http://master.invalid")
	if err := registry.UpdateNode(cluster.Snapshot{NodeID: "helper", NodeURL: helper.URL, ProtocolVersion: cluster.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	master.registry = registry
	master.scheduler.leaseBook.Replace([]offloadLease{{
		Lane:          cluster.RouteLaneText,
		OwnerNodeID:   "owner",
		OwnerModelID:  "owner-model",
		HelperNodeID:  "helper",
		HelperModelID: "helper-model",
		ExpiresAt:     time.Now().Add(time.Minute),
	}})
	relay := httptest.NewServer(http.HandlerFunc(master.handleNodeOffloadRequest))
	t.Cleanup(relay.Close)
	defer close(releaseHelper)

	request, err := http.NewRequest(http.MethodPost, relay.URL, strings.NewReader(`{"model":"owner-model","stream":true,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(offloadOwnerModelHeader, "owner-model")
	request.Header.Set(offloadOwnerHeader, "owner")
	request.Header.Set(offloadPathHeader, "/v1/chat/completions")
	request.Header.Set(offloadLaneHeader, cluster.RouteLaneText)
	firstEvent := make(chan string, 1)
	go func() {
		response, err := relay.Client().Do(request)
		if err != nil {
			firstEvent <- err.Error()
			return
		}
		defer response.Body.Close()
		line, _ := bufio.NewReader(response.Body).ReadString('\n')
		firstEvent <- line
	}()
	select {
	case line := <-firstEvent:
		if !strings.Contains(line, "first") {
			t.Fatalf("unexpected first relayed line %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay held the helper's first event until the helper finished")
	}
}
