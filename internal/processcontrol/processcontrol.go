package processcontrol

import (
	"context"
	"os/exec"
	"time"
)

type Options struct {
	HideWindow             bool
	ParentDeathGracePeriod time.Duration
}

func Start(cmd *exec.Cmd, options Options) error {
	Prepare(cmd, options)
	if err := cmd.Start(); err != nil {
		return err
	}
	Guard(cmd, options)
	return nil
}

func Stop(ctx context.Context, cmd *exec.Cmd, waitDone <-chan error, gracePeriod time.Duration, forceWait time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := Interrupt(cmd); err != nil {
		_ = Kill(cmd)
	}
	if waitDone == nil {
		_ = Kill(cmd)
		return nil
	}

	timer := time.NewTimer(gracePeriod)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		_ = Kill(cmd)
		waitAfterKill(waitDone, forceWait)
		return ctx.Err()
	case <-timer.C:
		_ = Kill(cmd)
		waitAfterKill(waitDone, forceWait)
		return nil
	case <-waitDone:
		return nil
	}
}

func ForceStop(cmd *exec.Cmd, waitDone <-chan error, forceWait time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := Kill(cmd)
	waitAfterKill(waitDone, forceWait)
	return err
}

func waitAfterKill(waitDone <-chan error, forceWait time.Duration) {
	if forceWait <= 0 {
		return
	}
	timer := time.NewTimer(forceWait)
	defer timer.Stop()
	select {
	case <-waitDone:
	case <-timer.C:
	}
}
