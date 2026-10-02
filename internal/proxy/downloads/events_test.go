package downloads

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
)

const eventTestJobID = "0123456789abcdef0123456789abcdef"

type interruptedSubscriptionService struct {
	downloader.Service
}

func (interruptedSubscriptionService) Job(string) (downloader.DownloadJob, bool, error) {
	return downloader.DownloadJob{ID: eventTestJobID, State: downloader.JobRunning}, true, nil
}

func (interruptedSubscriptionService) Subscribe(string) (<-chan downloader.DownloadJob, func()) {
	events := make(chan downloader.DownloadJob, 1)
	events <- downloader.DownloadJob{ID: eventTestJobID, State: downloader.JobRunning}
	close(events)
	return events, func() {}
}

func TestDownloadEventsEndWhenSubscriptionCloses(t *testing.T) {
	handlers := &Handlers{deps: localNodeDeps{}, downloader: interruptedSubscriptionService{}}
	recorder := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		handlers.NodeEvents(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/node/site/download/jobs/"+eventTestJobID+"/events", nil))
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("event stream kept running after the subscription closed")
	}
	if events := strings.Count(recorder.Body.String(), "event: progress"); events != 1 {
		t.Fatalf("expected exactly one progress event, got %d:\n%.300s", events, recorder.Body.String())
	}
}

func TestDownloadErrorStatusReflectsCause(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{&downloader.Error{Code: downloader.ErrorRateLimited, Message: "slow down"}, http.StatusTooManyRequests},
		{fmt.Errorf("wrapped: %w", &downloader.Error{Code: downloader.ErrorUpstreamUnavailable, Message: "hub down"}), http.StatusBadGateway},
		{&downloader.Error{Code: downloader.ErrorServiceUnavailable, Message: "restarting"}, http.StatusServiceUnavailable},
		{&downloader.Error{Code: downloader.ErrorNotFound, Message: "missing"}, http.StatusNotFound},
		{&cluster.RemoteError{StatusCode: http.StatusTooManyRequests}, http.StatusTooManyRequests},
		{errors.New("invalid selection"), http.StatusBadRequest},
	} {
		if status := downloadErrorStatus(test.err); status != test.status {
			t.Errorf("%v mapped to %d, want %d", test.err, status, test.status)
		}
	}
}
