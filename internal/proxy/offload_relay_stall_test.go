package proxy

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

const stallTestTimeout = 40 * time.Millisecond

func stallWatchWithHelper(working *atomic.Bool) *relayStallWatch {
	return newRelayStallWatch(
		func() time.Duration { return stallTestTimeout },
		func(context.Context, string) bool { return working.Load() },
		func(string, int) {},
	)
}

func mustBeCancelledByStall(t *testing.T, ctx context.Context, label string) {
	t.Helper()
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), errHelperStalled) {
			t.Fatalf("%s ended with %v, want the helper-stalled cause", label, context.Cause(ctx))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s was never reclaimed from the stalled helper", label)
	}
}

func mustStayRunning(t *testing.T, ctx context.Context, label string, duration time.Duration) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatalf("%s was cancelled (%v), want it left running", label, context.Cause(ctx))
	case <-time.After(duration):
	}
}

func TestSilentHelperWithNoBorrowedWorkLosesEveryLentRequestAtOnce(t *testing.T) {
	var working atomic.Bool
	watch := stallWatchWithHelper(&working)
	first, _ := watch.Begin(context.Background(), "slave")
	second, _ := watch.Begin(context.Background(), "slave")

	mustBeCancelledByStall(t, first, "first lent request")
	mustBeCancelledByStall(t, second, "second lent request")
}

func TestHelperStillRunningBorrowedWorkKeepsItsLargeRequests(t *testing.T) {
	var working atomic.Bool
	working.Store(true)
	watch := stallWatchWithHelper(&working)
	large, _ := watch.Begin(context.Background(), "slave")

	mustStayRunning(t, large, "large request on a helper that is generating", 5*stallTestTimeout)

	working.Store(false)
	mustBeCancelledByStall(t, large, "request once the helper stopped running borrowed work")
}

func TestAnyLentRequestAnsweringRestartsTheClockForTheRest(t *testing.T) {
	var working atomic.Bool
	watch := stallWatchWithHelper(&working)
	waiting, _ := watch.Begin(context.Background(), "slave")
	_, finishing := watch.Begin(context.Background(), "slave")

	time.Sleep(stallTestTimeout / 2)
	finishing.Answered()

	mustStayRunning(t, waiting, "request that waited while another was answered", stallTestTimeout*3/4)
	mustBeCancelledByStall(t, waiting, "request once the helper went quiet again")
}

func TestAnsweredRequestIsNeverReclaimed(t *testing.T) {
	var working atomic.Bool
	watch := stallWatchWithHelper(&working)
	answered, request := watch.Begin(context.Background(), "slave")
	request.Answered()

	mustStayRunning(t, answered, "answered request", 4*stallTestTimeout)
}

func TestStalledHelperDoesNotAffectAnotherHelpersRequests(t *testing.T) {
	var working atomic.Bool
	watch := stallWatchWithHelper(&working)
	stalled, _ := watch.Begin(context.Background(), "slave")
	other, _ := watch.Begin(context.Background(), "other-slave")
	_, finishing := watch.Begin(context.Background(), "other-slave")

	mustBeCancelledByStall(t, stalled, "stalled helper's request")
	finishing.Answered()
	mustBeCancelledByStall(t, other, "other helper's request after it also went quiet")
}
