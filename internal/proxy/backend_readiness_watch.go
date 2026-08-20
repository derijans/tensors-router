package proxy

import (
	"fmt"
	"sync"

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

// readinessWatch reports the first definitive verdict found in a backend's output.
type readinessWatch struct {
	mu      sync.Mutex
	scanner *backendreadiness.Scanner
	settled chan struct{}
	once    sync.Once
	stop    func()
	result  backendreadiness.Result
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
	watch.mu.Unlock()
	if result.Verdict != backendreadiness.Undecided {
		watch.once.Do(func() { close(watch.settled) })
	}
}

// settledChannel closes as soon as the output yields a definitive verdict.
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

func (watch *readinessWatch) close() {
	if watch == nil || watch.stop == nil {
		return
	}
	watch.stop()
}

// backendOutputVerdict converts a settled output verdict into a wait result. The second
// return reports whether the output decided the outcome; when false the caller keeps
// probing.
func (service *Service) backendOutputVerdict(watch *readinessWatch, modelID string, configFilename string) (error, bool) {
	result := watch.verdict()
	switch result.Verdict {
	case backendreadiness.Ready:
		service.logger.Printf("backend reported model ready model=%q config=%q", modelID, configFilename)
		return nil, true
	case backendreadiness.Failed:
		service.logger.Printf("backend reported load failure model=%q config=%q reason=%q", modelID, configFilename, result.Reason)
		return fmt.Errorf("backend reported load failure: %s", result.Reason), true
	default:
		return nil, false
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
