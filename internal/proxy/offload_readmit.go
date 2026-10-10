package proxy

import (
	"sync"
	"time"
)

type readmitAlarm struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func newReadmitAlarm() *readmitAlarm {
	return &readmitAlarm{timers: map[string]*time.Timer{}}
}

func (alarm *readmitAlarm) ring(lane string, at time.Time, readmit func()) {
	alarm.mu.Lock()
	defer alarm.mu.Unlock()
	wait := max(time.Until(at), time.Millisecond)
	if timer, armed := alarm.timers[lane]; armed {
		timer.Reset(wait)
		return
	}
	alarm.timers[lane] = time.AfterFunc(wait, readmit)
}

func (alarm *readmitAlarm) stop() {
	alarm.mu.Lock()
	defer alarm.mu.Unlock()
	for lane, timer := range alarm.timers {
		timer.Stop()
		delete(alarm.timers, lane)
	}
}
