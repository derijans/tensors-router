package vllm

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"

	"tensors-router/internal/processcontrol"
)

type CommandRunner interface {
	Run(context.Context, string, []string, []string, string, io.Writer) error
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, name string, arguments []string, environment []string, directory string, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	command := exec.Command(name, arguments...)
	command.Env = environment
	command.Dir = directory
	command.Stdout = output
	command.Stderr = output
	if err := processcontrol.Start(command, processcontrol.Options{HideWindow: true}); err != nil {
		return err
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- command.Wait()
	}()
	select {
	case err := <-waitDone:
		return err
	case <-ctx.Done():
		stopContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopError := processcontrol.Stop(stopContext, command, waitDone, 2*time.Second, 5*time.Second)
		if stopError != nil && !errors.Is(stopError, context.Canceled) && !errors.Is(stopError, context.DeadlineExceeded) {
			return errors.Join(ctx.Err(), stopError)
		}
		return ctx.Err()
	}
}
