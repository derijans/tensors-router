package proxy

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCoalescerFoldsEventsArrivingDuringADecisionIntoOneFollowUp(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var decided []string
	coalescer := newQueueEventCoalescer(func(_ context.Context, event queueEvent) {
		mu.Lock()
		decided = append(decided, event.Trigger)
		first := len(decided) == 1
		mu.Unlock()
		if first {
			<-release
		}
	})
	t.Cleanup(coalescer.Close)

	coalescer.Report(queueEvent{Lane: "image", ModelID: "model", Trigger: queueEventEnqueued})
	waitForDecisions(t, &mu, &decided, 1)
	coalescer.Report(queueEvent{Lane: "image", ModelID: "model", Trigger: queueEventEnqueued})
	coalescer.Report(queueEvent{Lane: "image", ModelID: "model", Trigger: queueEventCompleted})
	close(release)
	waitForDecisions(t, &mu, &decided, 2)
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(decided) != 2 || decided[1] != queueEventCompleted {
		t.Fatalf("decided = %v, want the first event and one follow-up carrying the newest trigger", decided)
	}
}

func TestCoalescerKeepsDifferentModelsIndependent(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var decided []string
	coalescer := newQueueEventCoalescer(func(_ context.Context, event queueEvent) {
		mu.Lock()
		decided = append(decided, event.ModelID)
		mu.Unlock()
		if event.ModelID == "blocked" {
			<-release
		}
	})
	t.Cleanup(func() {
		close(release)
		coalescer.Close()
	})

	coalescer.Report(queueEvent{Lane: "image", ModelID: "blocked", Trigger: queueEventEnqueued})
	coalescer.Report(queueEvent{Lane: "image", ModelID: "other", Trigger: queueEventEnqueued})

	waitForDecisions(t, &mu, &decided, 2)
}

func waitForDecisions(t *testing.T, mu *sync.Mutex, decided *[]string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(*decided)
		mu.Unlock()
		if count >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("only %d decisions made, want %d", len(*decided), want)
}
