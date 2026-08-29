package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
)

const txt2imgBody = `{"sd_model_checkpoint":"combo-dream","width":512,"height":512,"steps":30,"batch_size":1}`

// newGroupedImageService builds a node holding one image model, with a backend
// that blocks until the test releases it so queue states can be observed.
//
// The peer is declared as a group member but is deliberately not a routable
// replica: grouping is what makes the model queue in the router, and keeping
// selection local is what lets these tests observe that queue directly.
func newGroupedImageService(t *testing.T, gate chan struct{}, grouped bool) *Service {
	t.Helper()
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gate != nil {
			<-gate
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{}]}`))
	}), map[string]string{
		"combo": `{"nomodel":true,"sdmodel":"dream.safetensors"}`,
	})
	service.clusterToken = "secret"

	registry := cluster.NewRegistry(cluster.RoleStandalone, service.nodeID, "http://local")
	local := testClusterModel("combo", service.nodeID, "weights", "config-a", cluster.SourceLocal)
	local.HasLLM = false
	local.HasImage = true
	local.ImageID = "combo-dream"
	local.PublicImageID = "combo-dream"
	if err := registry.UpdateLocal([]cluster.Model{local}); err != nil {
		t.Fatal(err)
	}
	if grouped {
		registry.SetGroupSource(newRoutingGroupLookup([]routinggroups.Group{{
			ID: "group",
			Members: []routinggroups.Member{
				{NodeID: service.nodeID, ImageID: "combo-dream"},
				{NodeID: "slave-a", ImageID: "combo-alt-dream"},
			},
		}}))
	}
	service.registry = registry
	return service
}

func postImage(service *Service) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sdapi/v1/txt2img", strings.NewReader(txt2imgBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	return recorder
}

func postBorrowedImage(service *Service) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sdapi/v1/txt2img", strings.NewReader(txt2imgBody))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, markBorrowedRequest(request))
	return recorder
}

func waitForBacklog(t *testing.T, service *Service, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, stats := range service.imageQueue.Stats() {
			if stats.BacklogCount >= want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("queue never reached a backlog of %d: %+v", want, service.imageQueue.Stats())
}

// A grouped model routes through the router-held queue, and the queue reports the
// backlog a master needs in order to decide whether to lend it out. Without this
// the requests would already be inside the backend and could not be moved.
func TestGroupedImageRequestsQueueInTheRouter(t *testing.T) {
	gate := make(chan struct{})
	service := newGroupedImageService(t, gate, true)

	done := make(chan int, 3)
	for index := 0; index < 3; index++ {
		go func() { done <- postImage(service).Code }()
	}
	waitForBacklog(t, service, 3)

	stats := service.imageQueue.Stats()
	if len(stats) != 1 || stats[0].GroupID != "group" {
		t.Fatalf("stats = %+v, want one group", stats)
	}
	if stats[0].PendingCount == 0 {
		t.Fatalf("stats = %+v, want work still withdrawable behind the backend", stats)
	}

	close(gate)
	for index := 0; index < 3; index++ {
		if code := <-done; code != http.StatusOK {
			t.Fatalf("status %d, want 200", code)
		}
	}
}

// A model in no group must not be queued at all, so nothing changes for traffic
// that was never enrolled.
func TestUngroupedImageRequestsBypassTheQueue(t *testing.T) {
	service := newGroupedImageService(t, nil, false)

	if code := postImage(service).Code; code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if stats := service.imageQueue.Stats(); len(stats) != 0 {
		t.Fatalf("stats = %+v, want nothing queued for an ungrouped model", stats)
	}
}

// Borrowed work that has not started is handed back the moment this node has work
// of its own, and the owner sees a distinct code rather than a failure.
func TestBorrowedRequestIsReturnedWhileTheNodeHasItsOwnWork(t *testing.T) {
	gate := make(chan struct{})
	service := newGroupedImageService(t, gate, true)

	native := make(chan int, 1)
	go func() { native <- postImage(service).Code }()
	waitForBacklog(t, service, 1)

	recorder := postBorrowedImage(service)
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

// An idle node lends: borrowed work is served exactly like its own.
func TestIdleNodeServesBorrowedWork(t *testing.T) {
	service := newGroupedImageService(t, nil, true)

	if recorder := postBorrowedImage(service); recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s, want the borrowed request served", recorder.Code, recorder.Body.String())
	}
}

// The status a master polls has to describe both halves of the arrangement: what
// this node still has to do, and whether it can take anything more.
func TestRuntimeStatusReportsQueueAndBorrowingState(t *testing.T) {
	gate := make(chan struct{})
	service := newGroupedImageService(t, gate, true)

	done := make(chan int, 1)
	go func() { done <- postImage(service).Code }()
	waitForBacklog(t, service, 1)

	status := service.localRuntimeStatus()
	if status.AcceptingBorrowed {
		t.Fatal("node advertises it is accepting borrowed work while running its own")
	}
	if len(status.ImageQueue) != 1 || status.ImageQueue[0].GroupID != "group" {
		t.Fatalf("image queue = %+v, want the group backlog", status.ImageQueue)
	}

	close(gate)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if !service.localRuntimeStatus().AcceptingBorrowed {
		t.Fatal("node still refuses borrowed work after draining its own queue")
	}
}

// The marker is read once, at the cluster-token gated node entry, and deleted
// there. A client that sets the header itself must never be treated as borrowed,
// or it could make its own request preemptible and have it handed back.
func TestOffloadMarkerIsOnlyTrustedFromTheNodeEndpoint(t *testing.T) {
	plain := httptest.NewRequest(http.MethodPost, "/sdapi/v1/txt2img", strings.NewReader(txt2imgBody))
	plain.Header.Set(offloadMarkerHeader, "1")
	if requestIsBorrowed(plain) {
		t.Fatal("a client-supplied header marked the request as borrowed")
	}

	gate := make(chan struct{})
	service := newGroupedImageService(t, gate, true)
	native := make(chan int, 1)
	go func() { native <- postImage(service).Code }()
	waitForBacklog(t, service, 1)

	// Through the node endpoint the marker is honoured, so the request is handed
	// back rather than queued behind this node's own work.
	viaNode := httptest.NewRecorder()
	forwarded := httptest.NewRequest(http.MethodPost, "/router/v1/node/inference/sdapi/v1/txt2img", strings.NewReader(txt2imgBody))
	forwarded.Header.Set("Content-Type", "application/json")
	forwarded.Header.Set("Authorization", "Bearer secret")
	forwarded.Header.Set(offloadMarkerHeader, "1")
	service.ServeHTTP(viaNode, forwarded)
	if viaNode.Code != http.StatusConflict {
		t.Fatalf("node endpoint status %d body %s, want 409", viaNode.Code, viaNode.Body.String())
	}

	// The identical header straight from a client is ignored, so that request
	// queues normally and is served.
	fromClient := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/sdapi/v1/txt2img", strings.NewReader(txt2imgBody))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(offloadMarkerHeader, "1")
		service.ServeHTTP(recorder, request)
		fromClient <- recorder.Code
	}()
	waitForBacklog(t, service, 2)

	close(gate)
	if code := <-native; code != http.StatusOK {
		t.Fatalf("native status %d, want 200", code)
	}
	if code := <-fromClient; code != http.StatusOK {
		t.Fatalf("client request carrying the marker got %d, want it served normally", code)
	}
}
