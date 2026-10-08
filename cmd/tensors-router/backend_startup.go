package main

import (
	"context"
	"log"
	"strings"
	"time"

	"tensors-router/internal/config"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/proxy"
)

type startupStep struct {
	name  string
	phase loaderrors.Phase
	run   func(context.Context) error
}

type startupFailureRecorder func(phase loaderrors.Phase, source string, err error)

type backgroundStartup struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func startInBackground(parent context.Context, logger *log.Logger, steps []startupStep, recordFailure startupFailureRecorder) *backgroundStartup {
	ctx, cancel := context.WithCancel(parent)
	startup := &backgroundStartup{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(startup.done)
		for _, step := range steps {
			if ctx.Err() != nil {
				return
			}
			runStartupStep(ctx, logger, step, recordFailure)
		}
	}()
	return startup
}

func runStartupStep(ctx context.Context, logger *log.Logger, step startupStep, recordFailure startupFailureRecorder) {
	started := time.Now()
	logger.Printf("startup step started step=%q", step.name)
	err := step.run(ctx)
	switch {
	case err == nil:
		logger.Printf("startup step completed step=%q elapsed=%s", step.name, time.Since(started))
	case ctx.Err() != nil:
		logger.Printf("startup step cancelled step=%q elapsed=%s", step.name, time.Since(started))
	default:
		logger.Printf("startup step failed step=%q elapsed=%s error=%v", step.name, time.Since(started), err)
		recordFailure(step.phase, step.name, err)
	}
}

func (startup *backgroundStartup) Stop() {
	if startup == nil {
		return
	}
	startup.cancel()
	<-startup.done
}

func backendStartupSteps(cfg config.Config, families map[string]proxy.BackendFamilyConfig, router *proxy.Service) []startupStep {
	steps := []startupStep{}
	if family, ok := families[cfg.Backend.Mode]; ok && family.Start != nil {
		steps = append(steps, startupStep{name: "backend start mode=" + cfg.Backend.Mode, phase: loaderrors.PhaseStartup, run: family.Start})
	}
	if startupModel := strings.TrimSpace(cfg.Models.StartupModel); startupModel != "" {
		steps = append(steps, startupStep{name: "startup model preload model=" + startupModel, phase: loaderrors.PhasePreload, run: func(ctx context.Context) error {
			return router.PreloadModel(ctx, startupModel)
		}})
	}
	return steps
}
