package downloader

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	retryBaseDelay    = time.Second
	retryMaximumDelay = time.Minute
)

type retryWaiter func(ctx context.Context, delay time.Duration) error

func waitWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type upstreamStatusError struct {
	status     int
	retryAfter time.Duration
	message    string
}

func (problem upstreamStatusError) Error() string { return problem.message }

func (problem upstreamStatusError) Unwrap() error {
	return &Error{Code: errorCodeForStatus(problem.status), Message: problem.message}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func newUpstreamStatusError(response *http.Response, message string) upstreamStatusError {
	return upstreamStatusError{status: response.StatusCode, retryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()), message: message}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if moment, err := http.ParseTime(value); err == nil && moment.After(now) {
		return moment.Sub(now)
	}
	return 0
}

func backoffDelay(attempt int, err error) time.Duration {
	var status upstreamStatusError
	if errors.As(err, &status) && status.retryAfter > 0 {
		return min(status.retryAfter, retryMaximumDelay)
	}
	exponential := retryBaseDelay << min(attempt-1, 6)
	if exponential > retryMaximumDelay {
		exponential = retryMaximumDelay
	}
	jitter := time.Duration(rand.Int64N(int64(exponential)/4 + 1))
	return exponential - exponential/8 + jitter
}
