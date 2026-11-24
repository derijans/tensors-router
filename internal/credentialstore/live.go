package credentialstore

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"tensors-router/internal/credential"
)

const DefaultWatchInterval = 2 * time.Second

type Live struct {
	store     *Store
	logger    *log.Logger
	current   atomic.Pointer[Snapshot]
	refreshMu sync.Mutex
	mu        sync.Mutex
	listeners []func(*Snapshot)
}

func NewLive(ctx context.Context, store *Store, logger *log.Logger) (*Live, error) {
	if logger == nil {
		logger = log.Default()
	}
	live := &Live{store: store, logger: logger}
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	live.current.Store(snapshot)
	return live, nil
}

func (live *Live) Store() *Store {
	return live.store
}

func (live *Live) Current() *Snapshot {
	return live.current.Load()
}

func (live *Live) OnChange(listener func(*Snapshot)) {
	live.mu.Lock()
	defer live.mu.Unlock()
	live.listeners = append(live.listeners, listener)
}

func (live *Live) Refresh(ctx context.Context) error {
	live.refreshMu.Lock()
	defer live.refreshMu.Unlock()
	revision, err := live.store.Revision(ctx)
	if err != nil {
		return err
	}
	if revision == live.Current().Revision {
		return nil
	}
	snapshot, err := live.store.Snapshot(ctx)
	if err != nil {
		return err
	}
	live.current.Store(snapshot)
	live.notify(snapshot)
	return nil
}

func (live *Live) notify(snapshot *Snapshot) {
	live.mu.Lock()
	listeners := append([]func(*Snapshot){}, live.listeners...)
	live.mu.Unlock()
	for _, listener := range listeners {
		listener(snapshot)
	}
}

func (live *Live) Watch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := live.Refresh(ctx); err != nil && ctx.Err() == nil {
				live.logger.Printf("credential refresh failed: %v", err)
			}
		}
	}
}

func (live *Live) KeyHashes() credential.KeyHashes {
	return live.Current().Keys
}

func (live *Live) OutboundClusterToken() string {
	return live.Current().OutboundClusterToken
}

func (live *Live) AcceptsClusterToken(token string) bool {
	return token != "" && credential.MatchesAnyHash(token, live.Current().Keys.Cluster)
}
