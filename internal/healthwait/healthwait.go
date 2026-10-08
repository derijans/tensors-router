package healthwait

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const probeInterval = 500 * time.Millisecond

var ErrDeadline = errors.New("backend did not become healthy before the deadline")

type ExitError struct {
	Err error
}

func (err ExitError) Error() string {
	return fmt.Sprintf("backend exited before it became healthy: %v", err.Err)
}

func (err ExitError) Unwrap() error {
	return err.Err
}

func Wait(ctx context.Context, timeout time.Duration, exited <-chan error, healthy func(context.Context) bool) error {
	deadlineContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		select {
		case err := <-exited:
			return ExitError{Err: err}
		default:
		}
		if healthy(deadlineContext) {
			return nil
		}
		select {
		case <-deadlineContext.Done():
			if err := ctx.Err(); err != nil {
				return err
			}
			return ErrDeadline
		case err := <-exited:
			return ExitError{Err: err}
		case <-time.After(probeInterval):
		}
	}
}
