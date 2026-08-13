package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVLLMResponseStoreReleasesLeases(t *testing.T) {
	store := newVLLMResponseStore()
	defer store.close()
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	var releases atomic.Int32
	store.remember("resp-a", vllmResponseTarget{publicID: "public", nodeURL: "http://node-a"}, func() { releases.Add(1) })
	if target, found := store.target("resp-a"); !found || target.nodeURL != "http://node-a" {
		t.Fatalf("unexpected response target %#v found=%t", target, found)
	}
	now = now.Add(vllmResponseLifetime)
	if _, found := store.target("resp-a"); found {
		t.Fatal("expired response target remained available")
	}
	if releases.Load() != 1 {
		t.Fatalf("expired response lease released %d times", releases.Load())
	}
}

func TestVLLMResponseStoreExpiresLeaseWithoutLookup(t *testing.T) {
	store := newVLLMResponseStore(5 * time.Millisecond)
	defer store.close()
	base := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	var now atomic.Int64
	now.Store(base.UnixNano())
	store.now = func() time.Time { return time.Unix(0, now.Load()) }
	var releases atomic.Int32
	store.remember("resp-timed", vllmResponseTarget{}, func() { releases.Add(1) })
	now.Store(base.Add(vllmResponseLifetime).UnixNano())
	deadline := time.After(time.Second)
	for releases.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("expired response lease was not released by timer")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestVLLMResponseStoreCloseIsIdempotentAndRejectsNewLeases(t *testing.T) {
	store := newVLLMResponseStore(time.Hour)
	var releases atomic.Int32
	store.remember("resp-close", vllmResponseTarget{}, func() { releases.Add(1) })
	store.close()
	store.close()
	store.remember("resp-after-close", vllmResponseTarget{}, func() { releases.Add(1) })
	if releases.Load() != 2 {
		t.Fatalf("response leases released %d times", releases.Load())
	}
}

func TestVLLMResponseOperationEnforcesStreamingRequestLimit(t *testing.T) {
	service := NewService(ServiceConfig{})
	defer service.Close(t.Context())
	service.transportLimits.MaxRequestBytes = 4
	service.vllmResponses.remember("resp-limit", vllmResponseTarget{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses/resp-limit/cancel", strings.NewReader("12345"))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), `"type":"request_too_large"`) {
		t.Fatalf("unexpected oversized operation response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestVLLMResponseEventStreamFindsSplitResponseID(t *testing.T) {
	var responseID string
	stream := newVLLMResponseEventStream(io.NopCloser(strings.NewReader("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-123\"}}\n\n")), func(id string) {
		responseID = id
	})
	buffer := make([]byte, 7)
	for {
		_, err := stream.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if responseID != "resp-123" {
		t.Fatalf("split event response ID was not tracked: %q", responseID)
	}
}

func TestVLLMResponseTrackingPreservesJSONBody(t *testing.T) {
	service := NewService(ServiceConfig{})
	defer service.Close(t.Context())
	response := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(`{"id":"resp-7","status":"in_progress"}`)),
		ContentLength: -1,
	}
	tracked := service.responseWithVLLMTracking(response, vllmResponseTarget{publicID: "public"})
	body, err := io.ReadAll(tracked.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"id":"resp-7","status":"in_progress"}` {
		t.Fatalf("response body changed: %s", body)
	}
	if target, found := service.vllmResponses.target("resp-7"); !found || target.publicID != "public" {
		t.Fatalf("response ownership was not recorded: %#v found=%t", target, found)
	}
}
