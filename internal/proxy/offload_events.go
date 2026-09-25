package proxy

import (
	"context"
	"sync"
)

const (
	queueEventEnqueued          = "enqueued"
	queueEventCompleted         = "completed"
	queueEventBorrowedCompleted = "borrowed_completed"
	planTriggerTick             = "tick"
	planTriggerProbeDue         = "probe_due"
)

type queueEvent struct {
	Lane        string `json:"lane"`
	OwnerNodeID string `json:"owner_node_id"`
	ModelID     string `json:"model_id"`
	Trigger     string `json:"trigger"`
}

type queueEventDecision struct {
	Lease *offloadLease `json:"lease,omitempty"`
}

type queueEventCoalescer struct {
	decide  func(context.Context, queueEvent)
	ctx     context.Context
	cancel  context.CancelFunc
	running sync.WaitGroup

	mu       sync.Mutex
	inFlight map[string]*queueEvent
}

func newQueueEventCoalescer(decide func(context.Context, queueEvent)) *queueEventCoalescer {
	ctx, cancel := context.WithCancel(context.Background())
	return &queueEventCoalescer{decide: decide, ctx: ctx, cancel: cancel, inFlight: map[string]*queueEvent{}}
}

func (coalescer *queueEventCoalescer) Report(event queueEvent) {
	key := laneModelKey(event.Lane, event.ModelID)
	coalescer.mu.Lock()
	if coalescer.ctx.Err() != nil {
		coalescer.mu.Unlock()
		return
	}
	if followUp, busy := coalescer.inFlight[key]; busy {
		*followUp = event
		coalescer.mu.Unlock()
		return
	}
	coalescer.inFlight[key] = &queueEvent{}
	coalescer.running.Add(1)
	coalescer.mu.Unlock()
	go coalescer.drain(key, event)
}

func (coalescer *queueEventCoalescer) drain(key string, event queueEvent) {
	defer coalescer.running.Done()
	for {
		coalescer.decide(coalescer.ctx, event)
		coalescer.mu.Lock()
		followUp := coalescer.inFlight[key]
		if followUp.Trigger == "" || coalescer.ctx.Err() != nil {
			delete(coalescer.inFlight, key)
			coalescer.mu.Unlock()
			return
		}
		event = *followUp
		*followUp = queueEvent{}
		coalescer.mu.Unlock()
	}
}

func (coalescer *queueEventCoalescer) Close() {
	coalescer.mu.Lock()
	coalescer.cancel()
	coalescer.mu.Unlock()
	coalescer.running.Wait()
}
