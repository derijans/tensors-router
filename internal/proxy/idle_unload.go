package proxy

import (
	"context"
	"log"
	"sync"
	"time"
)

const idleUnloadSweepInterval = 5 * time.Second

type idleUnloadDeps interface {
	localRuntimes() []*backendRuntime
	idleUnloadTimeout(filename string) (time.Duration, error)
	borrowRestorePending(runtime *backendRuntime) bool
	unloadRuntimeWhile(ctx context.Context, runtime *backendRuntime, stillUnloadable func(*activeConfigState) error) error
}

type idleUnloader struct {
	deps       idleUnloadDeps
	interval   time.Duration
	now        func() time.Time
	logger     *log.Logger
	onUnloaded func(backendMode string)

	mu              sync.Mutex
	cancel          context.CancelFunc
	done            chan struct{}
	reportedInvalid map[string]string
}

func newIdleUnloader(deps idleUnloadDeps, logger *log.Logger, onUnloaded func(backendMode string)) *idleUnloader {
	return &idleUnloader{
		deps:            deps,
		interval:        idleUnloadSweepInterval,
		now:             time.Now,
		logger:          logger,
		onUnloaded:      onUnloaded,
		reportedInvalid: map[string]string{},
	}
}

func (unloader *idleUnloader) start(ctx context.Context) {
	unloader.mu.Lock()
	defer unloader.mu.Unlock()
	if unloader.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	unloader.cancel = cancel
	unloader.done = done
	go func() {
		defer close(done)
		unloader.run(ctx)
	}()
}

func (unloader *idleUnloader) run(ctx context.Context) {
	ticker := time.NewTicker(unloader.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			unloader.sweep(ctx)
		}
	}
}

func (unloader *idleUnloader) sweep(ctx context.Context) {
	for _, runtime := range unloader.deps.localRuntimes() {
		if ctx.Err() != nil {
			return
		}
		unloader.unloadIfIdle(ctx, runtime)
	}
}

func (unloader *idleUnloader) unloadIfIdle(ctx context.Context, runtime *backendRuntime) {
	snapshot, idle := idleRuntimeSnapshotOf(runtime.state)
	if !idle {
		return
	}
	timeout := unloader.configuredTimeout(snapshot.filename)
	now := unloader.now()
	if timeout <= 0 || now.Sub(snapshot.idleSince) < timeout {
		return
	}
	if unloader.deps.borrowRestorePending(runtime) {
		return
	}
	err := unloader.deps.unloadRuntimeWhile(ctx, runtime, idleUnloadStillDue(snapshot, timeout, now))
	switch {
	case err == nil:
		unloader.logger.Printf("idle unload runtime=%s config=%q idle=%s", runtime.name, snapshot.filename, now.Sub(snapshot.idleSince).Round(time.Second))
		unloader.onUnloaded(runtime.mode)
	case idleUnloadSkipped(err) || ctx.Err() != nil:
	default:
		unloader.logger.Printf("idle unload failed runtime=%s config=%q error=%v", runtime.name, snapshot.filename, err)
	}
}

func (unloader *idleUnloader) configuredTimeout(filename string) time.Duration {
	timeout, err := unloader.deps.idleUnloadTimeout(filename)
	unloader.mu.Lock()
	defer unloader.mu.Unlock()
	if err == nil {
		delete(unloader.reportedInvalid, filename)
		return timeout
	}
	if unloader.reportedInvalid[filename] != err.Error() {
		unloader.reportedInvalid[filename] = err.Error()
		unloader.logger.Printf("idle unload disabled config=%q error=%v", filename, err)
	}
	return 0
}

func (unloader *idleUnloader) close(ctx context.Context) error {
	unloader.mu.Lock()
	cancel, done := unloader.cancel, unloader.done
	unloader.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
