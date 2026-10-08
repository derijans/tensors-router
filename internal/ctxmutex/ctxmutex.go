package ctxmutex

import (
	"context"
	"sync"
)

type Mutex struct {
	once sync.Once
	slot chan struct{}
}

func (mutex *Mutex) Lock(ctx context.Context) error {
	mutex.once.Do(mutex.initialize)
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mutex.slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (mutex *Mutex) Unlock() {
	mutex.once.Do(mutex.initialize)
	<-mutex.slot
}

func (mutex *Mutex) initialize() {
	mutex.slot = make(chan struct{}, 1)
}
