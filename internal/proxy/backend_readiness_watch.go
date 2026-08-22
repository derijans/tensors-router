package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tensors-router/internal/backendreadiness"
	"tensors-router/internal/loadcapture"
)

// backendOutputWatcher is implemented by backends that can stream their process output
// live. Watching the output lets the router tell "still loading" apart from "the process
// died and came back with no model" — a distinction the HTTP readiness probe cannot make,
// because both report a model id of "inactive".
type backendOutputWatcher interface {
	WatchOutput(observe func(loadcapture.Stream, []byte)) func()
}

// readinessWatch reports the first definitive verdict found in a backend's output, and
// tracks when that output last arrived.
type readinessWatch struct {
	mu       sync.Mutex
	scanner  *backendreadiness.Scanner
	settled  chan struct{}
	once     sync.Once
	stop     func()
	result   backendreadiness.Result
	lastSeen time.Time
}

// watchBackendReadiness begins watching runtime's output for load markers. It returns nil
// when the backend cannot stream output, in which case callers fall back to probing.
// The caller must call close on a non-nil result.
func (service *Service) watchBackendReadiness(runtime *backendRuntime, readiness backendReadiness) *readinessWatch {
	if runtime == nil || runtime.backend == nil {
		return nil
	}
	watcher, ok := runtime.backend.(backendOutputWatcher)
	if !ok {
		return nil
	}
	family := backendreadiness.FamilyNative
	if runtime.mode == BackendModeKobold {
		family = backendreadiness.FamilyKobold
	}
	watch := &readinessWatch{
		scanner: backendreadiness.NewScanner(family, readinessOutputLane(readiness)),
		settled: make(chan struct{}),
	}
	watch.stop = watcher.WatchOutput(watch.observe)
	return watch
}

func (watch *readinessWatch) observe(_ loadcapture.Stream, payload []byte) {
	watch.mu.Lock()
	result := watch.scanner.Write(payload)
	watch.result = result
	watch.lastSeen = time.Now()
	watch.mu.Unlock()
	if result.Verdict != backendreadiness.Undecided {
		watch.once.Do(func() { close(watch.settled) })
	}
}

// settledChannel closes as soon as the output reaches any definitive verdict. It latches,
// so it is only useful as a one-shot gate — see awaitOutputGate.
func (watch *readinessWatch) settledChannel() <-chan struct{} {
	if watch == nil {
		return nil
	}
	return watch.settled
}

func (watch *readinessWatch) verdict() backendreadiness.Result {
	if watch == nil {
		return backendreadiness.Result{}
	}
	watch.mu.Lock()
	defer watch.mu.Unlock()
	return watch.result
}

// loading reports whether the backend has announced a load in progress. Measured: while
// loading, both backends print progress and eventually a completion marker; with no model
// they print neither and serve "no model" forever.
func (watch *readinessWatch) loading() bool {
	if watch == nil {
		return false
	}
	watch.mu.Lock()
	defer watch.mu.Unlock()
	return watch.scanner.Loading()
}

// lastActivity reports when the backend last produced output, or the zero time if it has
// produced none. A load that is still printing is still working, however slowly.
func (watch *readinessWatch) lastActivity() time.Time {
	if watch == nil {
		return time.Time{}
	}
	watch.mu.Lock()
	defer watch.mu.Unlock()
	return watch.lastSeen
}

func (watch *readinessWatch) close() {
	if watch == nil || watch.stop == nil {
		return
	}
	watch.stop()
}

// backendOutputFailure reports a load failure the backend announced on its own output.
//
// Only failure is taken from the output, never readiness. The markers identify a lane,
// not a model: any "Load Text Model OK: True" satisfies any text-lane watcher, and
// waitForInactiveBackend calls into here from the retry path without holding the switch
// lock, so a banner from one load can be observed by the next one. Treating that as
// "ready" declares a model loaded that is not, and the request then hits an unloaded
// backend. A failure marker does not have that problem: it reports that the process is
// serving no model in this lane at all, which is true for every waiter on it.
//
// Readiness therefore stays with the HTTP probe, which reflects current state rather
// than a historical log line.
func (service *Service) backendOutputFailure(watch *readinessWatch, modelID string, configFilename string) error {
	result := watch.verdict()
	if result.Verdict != backendreadiness.Failed {
		return nil
	}
	service.logger.Printf("backend reported load failure model=%q config=%q reason=%q", modelID, configFilename, result.Reason)
	return fmt.Errorf("backend reported load failure: %s", result.Reason)
}

// awaitOutputGate holds off probing until the backend's own output says it is worth
// probing. The router captures that output in memory regardless of logging settings, so
// this costs nothing extra to read.
//
// The output opens the gate; it never answers the question. An early or stale marker only
// means probing starts sooner, and the probe still has to agree — which is what keeps a
// lane-scoped banner from declaring some other model's load complete. A failure marker
// ends the wait outright, so a backend that cannot come up is not polled to death.
//
// The gate is bounded: a backend that prints nothing recognisable must not stop the
// router from probing at all.
func (service *Service) awaitOutputGate(watch *readinessWatch, ctx context.Context, gate time.Duration, modelID string, configFilename string) error {
	if watch == nil {
		return nil
	}
	timer := time.NewTimer(gate)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-watch.settledChannel():
		if err := service.backendOutputFailure(watch, modelID, configFilename); err != nil {
			return err
		}
		service.logger.Printf("backend output reports load complete; probing model=%q config=%q", modelID, configFilename)
		return nil
	case <-timer.C:
		// Nothing recognisable was printed in time. Fall through and probe anyway.
		return nil
	}
}

func readinessOutputLane(readiness backendReadiness) backendreadiness.Lane {
	switch readiness {
	case readinessImage:
		return backendreadiness.LaneImage
	case readinessEmbeddings:
		return backendreadiness.LaneEmbeddings
	case readinessSpeech:
		return backendreadiness.LaneSpeech
	case readinessTranscription:
		return backendreadiness.LaneTranscription
	case readinessMusic:
		return backendreadiness.LaneMusic
	default:
		return backendreadiness.LaneText
	}
}
