package proxy

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"tensors-router/internal/loadcapture"
)

type backendLoadCaptureRecorder interface {
	BeginLoadCapture(int64) func() loadcapture.Capture
}

type physicalLoadCapture struct {
	attempt      loadcapture.Attempt
	finishOutput func() loadcapture.Capture
	redactions   map[string]string
}

func (service *Service) beginPhysicalLoadCapture(ctx context.Context, runtime *backendRuntime, configFilename string, readiness backendReadiness) (*physicalLoadCapture, error) {
	if service.loadCaptureStore == nil {
		return nil, nil
	}
	snapshot, err := loadcapture.BuildSnapshot(filepath.Join(service.configDir, configFilename))
	if err != nil {
		return nil, err
	}
	attempt, err := service.loadCaptureStore.BeginPhysical(ctx, snapshot, runtime.mode, runtime.name, readinessAnalyticsSection(readiness))
	if err != nil {
		service.logger.Printf("load capture start write failed error=%v", err)
		return nil, nil
	}
	finishOutput := func() loadcapture.Capture { return loadcapture.Capture{} }
	if recorder, ok := runtime.backend.(backendLoadCaptureRecorder); ok {
		finishOutput = recorder.BeginLoadCapture(service.loadCaptureMaxOutputBytes)
	}
	return &physicalLoadCapture{attempt: attempt, finishOutput: finishOutput, redactions: snapshot.Redactions}, nil
}

func (service *Service) finishPhysicalLoadCapture(capture *physicalLoadCapture, loadErr error) {
	if capture == nil || service.loadCaptureStore == nil {
		return
	}
	context, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := service.loadCaptureStore.CompletePhysical(context, capture.attempt, loadErr, capture.finishOutput(), capture.redactions); err != nil {
		service.logger.Printf("load capture write failed attempt=%q error=%v", capture.attempt.ID, err)
	}
}

func (service *Service) recordLoadReuse(physicalAttemptID string) {
	if service.loadCaptureStore == nil || strings.TrimSpace(physicalAttemptID) == "" {
		return
	}
	context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := service.loadCaptureStore.RecordReuse(context, physicalAttemptID); err != nil {
		service.logger.Printf("load capture reuse write failed physical_attempt=%q error=%v", physicalAttemptID, err)
	}
}
