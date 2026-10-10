package proxy

import (
	"context"
	"time"
)

type lentRequestBound struct {
	cancel context.CancelFunc
	timer  *time.Timer
}

func (scheduler *scheduler) boundLentRequest(parent context.Context, entry *offloadEntry) (context.Context, *lentRequestBound) {
	bounded, cancel := context.WithCancel(parent)
	bound := &lentRequestBound{cancel: cancel}
	if giveUpAfter := scheduler.lent.GiveUpAfter(entry); giveUpAfter > 0 {
		bound.timer = time.AfterFunc(giveUpAfter, cancel)
	}
	return bounded, bound
}

func (bound *lentRequestBound) answered() {
	if bound.timer != nil {
		bound.timer.Stop()
	}
}

func (bound *lentRequestBound) release() {
	bound.answered()
	bound.cancel()
}
