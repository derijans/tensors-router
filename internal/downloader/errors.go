package downloader

import (
	"errors"
	"net/http"
)

type ErrorCode string

const (
	ErrorRateLimited         ErrorCode = "rate_limited"
	ErrorUpstreamUnavailable ErrorCode = "upstream_unavailable"
	ErrorDenied              ErrorCode = "denied"
	ErrorNotFound            ErrorCode = "not_found"
	ErrorServiceUnavailable  ErrorCode = "service_unavailable"
)

type Error struct {
	Code    ErrorCode
	Message string
}

func (problem *Error) Error() string { return problem.Message }

func ErrorCodeOf(err error) ErrorCode {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

func errorCodeForStatus(status int) ErrorCode {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrorDenied
	case status == http.StatusNotFound:
		return ErrorNotFound
	case status == http.StatusTooManyRequests:
		return ErrorRateLimited
	case status >= 500:
		return ErrorUpstreamUnavailable
	}
	return ""
}
