package main

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"tensors-router/internal/loaderrors"
)

func TestBackgroundStartupStopCancelsAStartThatNeverFinishes(t *testing.T) {
	cancelled := make(chan struct{})
	startup := startInBackground(context.Background(), log.New(io.Discard, "", 0), []startupStep{{
		name: "backend start",
		run: func(ctx context.Context) error {
			<-ctx.Done()
			close(cancelled)
			return ctx.Err()
		},
	}}, func(loaderrors.Phase, string, error) { t.Error("a cancelled start was recorded as a failure") })

	stopped := make(chan struct{})
	go func() {
		startup.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not end the hanging start")
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("stop returned before the start observed its cancellation")
	}
}

func TestBackgroundStartupRecordsAFailureAndRunsTheNextStep(t *testing.T) {
	failure := errors.New("koboldcpp did not become healthy")
	var recorded []string
	preloaded := false
	startup := startInBackground(context.Background(), log.New(io.Discard, "", 0), []startupStep{
		{name: "backend start", phase: loaderrors.PhaseStartup, run: func(context.Context) error { return failure }},
		{name: "startup model preload", phase: loaderrors.PhasePreload, run: func(context.Context) error { preloaded = true; return nil }},
	}, func(phase loaderrors.Phase, source string, err error) {
		recorded = append(recorded, string(phase)+" "+source+" "+err.Error())
	})
	<-startup.done

	if len(recorded) != 1 || recorded[0] != "startup backend start koboldcpp did not become healthy" || !preloaded {
		t.Fatalf("recorded=%q preloaded=%t", recorded, preloaded)
	}
}
