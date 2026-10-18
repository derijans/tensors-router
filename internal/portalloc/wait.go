package portalloc

import (
	"context"
	"time"
)

const availabilityPollInterval = 100 * time.Millisecond

func WaitForRelease(ctx context.Context, host, port string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(availabilityPollInterval)
	defer poll.Stop()
	for {
		err := CheckAvailable(host, port)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-deadline.C:
			return err
		case <-poll.C:
		}
	}
}
