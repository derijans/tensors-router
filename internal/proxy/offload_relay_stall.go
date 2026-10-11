package proxy

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errHelperStalled = errors.New("helper showed no progress on the requests lent to it")

type relayedRequest struct {
	watch    *relayStallWatch
	helperID string
	cancel   context.CancelCauseFunc
}

type watchedHelper struct {
	unanswered map[*relayedRequest]struct{}
	progress   uint64
	timer      *time.Timer
}

type relayStallWatch struct {
	mu            sync.Mutex
	helpers       map[string]*watchedHelper
	timeout       func() time.Duration
	helperWorking func(ctx context.Context, helperNodeID string) bool
	onReclaimed   func(helperNodeID string, reclaimed int)
}

func newRelayStallWatch(timeout func() time.Duration, helperWorking func(ctx context.Context, helperNodeID string) bool, onReclaimed func(helperNodeID string, reclaimed int)) *relayStallWatch {
	return &relayStallWatch{
		helpers:       map[string]*watchedHelper{},
		timeout:       timeout,
		helperWorking: helperWorking,
		onReclaimed:   onReclaimed,
	}
}

func (watch *relayStallWatch) Begin(parent context.Context, helperNodeID string) (context.Context, *relayedRequest) {
	ctx, cancel := context.WithCancelCause(parent)
	request := &relayedRequest{watch: watch, helperID: helperNodeID, cancel: cancel}
	watch.mu.Lock()
	defer watch.mu.Unlock()
	helper := watch.helperLocked(helperNodeID)
	helper.unanswered[request] = struct{}{}
	watch.noteProgressLocked(helperNodeID, helper)
	return ctx, request
}

func (request *relayedRequest) Answered() {
	request.watch.settle(request)
}

func (request *relayedRequest) End() {
	request.watch.settle(request)
	request.cancel(nil)
}

func (watch *relayStallWatch) settle(request *relayedRequest) {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	helper, watched := watch.helpers[request.helperID]
	if !watched {
		return
	}
	delete(helper.unanswered, request)
	watch.noteProgressLocked(request.helperID, helper)
}

func (watch *relayStallWatch) helperLocked(helperNodeID string) *watchedHelper {
	helper, watched := watch.helpers[helperNodeID]
	if !watched {
		helper = &watchedHelper{unanswered: map[*relayedRequest]struct{}{}}
		watch.helpers[helperNodeID] = helper
	}
	return helper
}

func (watch *relayStallWatch) noteProgressLocked(helperNodeID string, helper *watchedHelper) {
	helper.progress++
	if len(helper.unanswered) == 0 {
		if helper.timer != nil {
			helper.timer.Stop()
		}
		delete(watch.helpers, helperNodeID)
		return
	}
	if helper.timer == nil {
		helper.timer = time.AfterFunc(watch.timeout(), func() { watch.expired(helperNodeID) })
		return
	}
	helper.timer.Reset(watch.timeout())
}

func (watch *relayStallWatch) expired(helperNodeID string) {
	watch.mu.Lock()
	helper, watched := watch.helpers[helperNodeID]
	if !watched || len(helper.unanswered) == 0 {
		watch.mu.Unlock()
		return
	}
	progressBeforeProbe := helper.progress
	watch.mu.Unlock()

	probeContext, stopProbe := context.WithTimeout(context.Background(), stallProbeTimeout)
	working := watch.helperWorking(probeContext, helperNodeID)
	stopProbe()

	watch.mu.Lock()
	defer watch.mu.Unlock()
	helper, watched = watch.helpers[helperNodeID]
	if !watched || len(helper.unanswered) == 0 {
		return
	}
	if working || helper.progress != progressBeforeProbe {
		helper.timer.Reset(watch.timeout())
		return
	}
	reclaimed := len(helper.unanswered)
	for request := range helper.unanswered {
		request.cancel(errHelperStalled)
	}
	helper.unanswered = map[*relayedRequest]struct{}{}
	watch.onReclaimed(helperNodeID, reclaimed)
}

const stallProbeTimeout = 10 * time.Second
