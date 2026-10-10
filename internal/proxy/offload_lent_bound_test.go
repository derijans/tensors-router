package proxy

import (
	"context"
	"testing"
	"time"
)

func lendOne(scheduler *scheduler, giveUpAfter time.Duration) *offloadEntry {
	entry := holdNativeImageRequests(scheduler, 1)[0]
	scheduler.lent.Add(entry, lentRequest{lane: "image", modelID: lentModelID, giveUpAfter: giveUpAfter})
	return entry
}

func TestLentRequestIsCutOffWhenTheHelperNeverAnswers(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entry := lendOne(scheduler, 20*time.Millisecond)

	bounded, bound := scheduler.boundLentRequest(context.Background(), entry)
	defer bound.release()

	select {
	case <-bounded.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a lent request the helper never answers was never cut off")
	}
}

func TestLentRequestTheHelperAnsweredIsNotCutOffWhileItsBodyIsRead(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entry := lendOne(scheduler, 30*time.Millisecond)

	bounded, bound := scheduler.boundLentRequest(context.Background(), entry)
	bound.answered()

	select {
	case <-bounded.Done():
		t.Fatal("the deadline cut off a response the helper had already started sending")
	case <-time.After(150 * time.Millisecond):
	}
	bound.release()
	if bounded.Err() == nil {
		t.Fatal("releasing the bound left the request context running")
	}
}

func TestLentRequestWithoutAPredictionHasNoDeadline(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entry := lendOne(scheduler, 0)

	bounded, bound := scheduler.boundLentRequest(context.Background(), entry)
	defer bound.release()

	select {
	case <-bounded.Done():
		t.Fatal("a lent request with no predicted duration was cut off")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestLentRequestEndsWithItsClient(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entry := lendOne(scheduler, time.Hour)
	client, leave := context.WithCancel(context.Background())

	bounded, bound := scheduler.boundLentRequest(client, entry)
	defer bound.release()
	leave()

	select {
	case <-bounded.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a lent request outlived the client that sent it")
	}
}
