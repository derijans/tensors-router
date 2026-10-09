package proxy

import (
	"context"
	"fmt"
	"time"
)

const readinessWaitLogInterval = 30

type readinessWait struct {
	service           *Service
	runtime           *backendRuntime
	ctx               context.Context
	readiness         backendReadiness
	modelID           string
	configFilename    string
	watch             *readinessWatch
	progressIdleLimit time.Duration
	lastProgress      time.Time
	lastStatus        int
	lastBody          string
	lastErr           error
}

// waitForBackendEndpointWatching takes a watch registered before the load began, so no
// startup output is missed. A watch started after the reload can miss the very line it is
// waiting for, and then the gate just burns its timeout.
func (service *Service) waitForBackendEndpointWatching(runtime *backendRuntime, ctx context.Context, readiness backendReadiness, modelID string, configFilename string, watch *readinessWatch) error {
	// Readiness is decided by the HTTP probe, which reflects current state. The backend's
	// output runs alongside it to catch definitive failures early, because a probe alone
	// cannot tell a slow load from a dead one — both report a model id of "inactive" —
	// and waiting out a failed launch is exactly the stall this is here to avoid.
	wait := readinessWait{
		service:           service,
		runtime:           runtime,
		ctx:               ctx,
		readiness:         readiness,
		modelID:           modelID,
		configFilename:    configFilename,
		watch:             watch,
		progressIdleLimit: service.backendReadinessTimeout(),
		lastProgress:      time.Now(),
	}
	if err := service.awaitOutputGate(watch, ctx, backendOutputGraceWait, modelID, configFilename); err != nil {
		return err
	}
	retryDelay := service.backendRetryDelay
	for attempt := 1; attempt <= service.backendRetryAttempts; attempt++ {
		ready, err := wait.probe(attempt)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if wait.stoppedMakingProgress(attempt) {
			break
		}
		if attempt < service.backendRetryAttempts {
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return err
			}
			retryDelay = nextRetryDelay(retryDelay, service.backendRetryMaxDelay)
		}
	}
	return wait.failure()
}

func (wait *readinessWait) probe(attempt int) (bool, error) {
	service := wait.service
	if reporter, ok := wait.runtime.backend.(backendExitReporter); ok {
		if err := reporter.BackendExitError(); err != nil {
			return false, err
		}
	}
	if err := service.backendOutputFailure(wait.watch, wait.modelID, wait.configFilename); err != nil {
		return false, err
	}
	status, body, err := service.probeBackendEndpoint(wait.runtime, wait.ctx, wait.readiness.endpointForMode(wait.runtime.mode))
	if err == nil && backendEndpointReady(wait.readiness, wait.runtime.mode, status, body) {
		if attempt > 1 {
			service.logger.Printf("backend model endpoint ready model=%q config=%q attempt=%d", wait.modelID, wait.configFilename, attempt)
		}
		return true, nil
	}
	wait.lastStatus, wait.lastBody, wait.lastErr = status, body, err
	if attempt == 1 || attempt%readinessWaitLogInterval == 0 {
		service.logger.Printf("waiting for backend model endpoint model=%q config=%q attempt=%d status=%d error=%v body=%q", wait.modelID, wait.configFilename, attempt, status, err, body)
	}
	return false, nil
}

func (wait *readinessWait) stoppedMakingProgress(attempt int) bool {
	if seen := wait.watch.lastActivity(); seen.After(wait.lastProgress) {
		wait.lastProgress = seen
	}
	// A backend that announced a load is working, however long it takes — a cold
	// multi-gigabyte load measured here ran well past two minutes with long silent
	// stretches. Only a backend that never announced one, and is answering that it
	// holds nothing, has actually stopped: that is a reload that was accepted and
	// lost, and it is reported so the caller can re-issue it.
	if wait.watch.loading() || !probeAnswersWithoutModel(wait.readiness, wait.lastStatus, wait.lastBody) || time.Since(wait.lastProgress) <= wait.progressIdleLimit {
		return false
	}
	wait.service.logger.Printf("backend is serving no model and no load is in progress model=%q config=%q attempt=%d", wait.modelID, wait.configFilename, attempt)
	return true
}

func (wait *readinessWait) failure() error {
	if err := wait.service.backendOutputFailure(wait.watch, wait.modelID, wait.configFilename); err != nil {
		return err
	}
	if probeReportsNoModel(wait.lastStatus, wait.lastBody) {
		return fmt.Errorf("%w: status %d body=%q", errBackendServingNoModel, wait.lastStatus, wait.lastBody)
	}
	return fmt.Errorf("backend model endpoint unavailable after retries: status %d error=%v body=%q", wait.lastStatus, wait.lastErr, wait.lastBody)
}
