package proxy

import (
	"context"
	"net/http"
)

type backendRetrySession struct {
	service      *Service
	runtime      *backendRuntime
	ctx          context.Context
	modelContext context.Context
	original     *http.Request
	body         []byte
	target       backendForwardTarget
	releaseModel func()
	recovered    bool
}

func (session *backendRetrySession) forward(loadedFresh bool) (*http.Response, error) {
	response, err := session.service.forward(session.runtime, session.ctx, session.original, session.body)
	result := session.service.backendRetryResult(response, err, session.path())
	if !result.retry {
		return responseWithRelease(response, session.releaseModel), nil
	}
	session.logRetryable(0, result)
	recoveredNow, err := session.recoverIfTransportFailed(result)
	if err != nil {
		return nil, err
	}
	if !recoveredNow {
		session.logRetryWithoutRecovery(loadedFresh, result)
	}
	return session.retryUntilExhausted(result)
}

func (session *backendRetrySession) retryUntilExhausted(result backendRetryResult) (*http.Response, error) {
	service := session.service
	retryDelay := service.backendRetryDelay
	skipRetryDelay, err := session.waitIfInactive(result)
	if err != nil {
		return nil, err
	}
	for attempt := 1; attempt <= service.inferenceRetryAttempts(result); attempt++ {
		if attempt > 1 && !skipRetryDelay {
			if err := waitForRetry(session.ctx, retryDelay); err != nil {
				session.releaseModel()
				return nil, err
			}
			retryDelay = nextRetryDelay(retryDelay, service.backendRetryMaxDelay)
		}
		response, forwardErr := service.forward(session.runtime, session.ctx, session.original, session.body)
		result = service.backendRetryResult(response, forwardErr, session.path())
		if !result.retry {
			return responseWithRelease(response, session.releaseModel), nil
		}
		session.logRetryable(attempt, result)
		if _, err := session.recoverIfTransportFailed(result); err != nil {
			return nil, err
		}
		if skipRetryDelay, err = session.waitIfInactive(result); err != nil {
			return nil, err
		}
	}
	return session.exhausted(result)
}

func (session *backendRetrySession) recoverIfTransportFailed(result backendRetryResult) (bool, error) {
	if session.recovered || !session.service.shouldRecoverBackend(session.runtime, session.ctx, result) {
		return false, nil
	}
	target := session.target
	logger := session.service.logger
	logger.Printf("backend transport recovery attempt path=%s model=%q config=%q error=%v", session.path(), target.modelID, target.configFilename, result.err)
	if err := session.service.reloadHeldModelConfig(session.runtime, session.modelContext, target.modelID, target.configFilename, target.readiness); err != nil {
		session.releaseModel()
		return false, err
	}
	logger.Printf("backend transport recovery succeeded path=%s model=%q config=%q reloaded=true", session.path(), target.modelID, target.configFilename)
	session.recovered = true
	return true, nil
}

func (session *backendRetrySession) waitIfInactive(result backendRetryResult) (bool, error) {
	if !result.inactive {
		return false, nil
	}
	target := session.target
	if err := session.service.waitForInactiveBackend(session.runtime, session.modelContext, target.readiness, target.modelID, target.configFilename, session.path()); err != nil {
		session.releaseModel()
		return false, err
	}
	return true, nil
}

func (session *backendRetrySession) exhausted(result backendRetryResult) (*http.Response, error) {
	target := session.target
	session.service.logger.Printf("backend retry exhausted path=%s model=%q config=%q attempts=%d status=%d error=%v body=%q", session.path(), target.modelID, target.configFilename, session.service.inferenceRetryAttempts(result), result.status, result.err, result.body)
	if result.emptyOutput {
		return responseWithRelease(emptyOutputResponse(result), session.releaseModel), nil
	}
	session.releaseModel()
	return nil, backendRetryExhaustedError(result.status, result.err, result.body)
}

func (session *backendRetrySession) logRetryable(attempt int, result backendRetryResult) {
	session.service.logRetryableBackendResult(session.path(), session.target.modelID, session.target.configFilename, attempt, result.status, result.err, result.body)
}

func (session *backendRetrySession) logRetryWithoutRecovery(loadedFresh bool, result backendRetryResult) {
	target := session.target
	logger := session.service.logger
	switch {
	case !loadedFresh && !isBackendWaitingResponse(result.status, result.body):
		logger.Printf("backend returned a retryable response; retrying without reload model=%q config=%q status=%d", target.modelID, target.configFilename, result.status)
	case !loadedFresh:
		logger.Printf("backend unavailable while config already active; retrying without reload model=%q config=%q", target.modelID, target.configFilename)
	default:
		logger.Printf("backend retry after fresh config load model=%q config=%q", target.modelID, target.configFilename)
	}
}

func (session *backendRetrySession) path() string {
	return session.original.URL.Path
}
