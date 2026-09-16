package proxy

import (
	"context"
	"net/http"
)

type borrowLoadAllowedContextKey struct{}

func markBorrowLoadAllowed(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), borrowLoadAllowedContextKey{}, true))
}

func contextBorrowLoadAllowed(ctx context.Context) bool {
	allowed, _ := ctx.Value(borrowLoadAllowedContextKey{}).(bool)
	return allowed
}

func (service *Service) borrowedRequestWouldLoadWithoutPermission(r *http.Request, mode string, readiness backendReadiness, configFilename string) bool {
	if !requestIsBorrowed(r) || contextBorrowLoadAllowed(r.Context()) {
		return false
	}
	runtime, err := service.runtimeForBackendMode(mode, readiness)
	if err != nil || runtime == nil {
		return true
	}
	return currentRuntimeConfigFilename(runtime) != configFilename
}
