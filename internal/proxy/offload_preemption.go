package proxy

import (
	"context"
	"net/http"
	"sync"
)

type borrowedPreemptionKey struct{}

type borrowedPreemption struct {
	mu        sync.Mutex
	started   bool
	preempted chan struct{}
}

func withBorrowedPreemption(r *http.Request) *http.Request {
	preemption := &borrowedPreemption{preempted: make(chan struct{})}
	return r.WithContext(context.WithValue(r.Context(), borrowedPreemptionKey{}, preemption))
}

func contextBorrowedPreemption(ctx context.Context) *borrowedPreemption {
	preemption, _ := ctx.Value(borrowedPreemptionKey{}).(*borrowedPreemption)
	return preemption
}

func (preemption *borrowedPreemption) claimStart() bool {
	if preemption == nil {
		return true
	}
	preemption.mu.Lock()
	defer preemption.mu.Unlock()
	if preemption.wasPreemptedLocked() {
		return false
	}
	preemption.started = true
	return true
}

func (preemption *borrowedPreemption) preemptUnlessStarted() {
	if preemption == nil {
		return
	}
	preemption.mu.Lock()
	defer preemption.mu.Unlock()
	if preemption.started || preemption.wasPreemptedLocked() {
		return
	}
	close(preemption.preempted)
}

func (preemption *borrowedPreemption) wasPreempted() bool {
	if preemption == nil {
		return false
	}
	preemption.mu.Lock()
	defer preemption.mu.Unlock()
	return preemption.wasPreemptedLocked()
}

func (preemption *borrowedPreemption) wasPreemptedLocked() bool {
	select {
	case <-preemption.preempted:
		return true
	default:
		return false
	}
}

func (preemption *borrowedPreemption) preemptedSignal() <-chan struct{} {
	if preemption == nil {
		return nil
	}
	return preemption.preempted
}

func contextEndingWhenPreempted(ctx context.Context, preemption *borrowedPreemption) (context.Context, context.CancelFunc) {
	signal := preemption.preemptedSignal()
	if signal == nil {
		return ctx, func() {}
	}
	bound, cancel := context.WithCancelCause(ctx)
	go func() {
		select {
		case <-signal:
			cancel(errOffloadReturned)
		case <-bound.Done():
		}
	}()
	return bound, func() { cancel(nil) }
}
