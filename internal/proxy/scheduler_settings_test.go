package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"

	"tensors-router/internal/offloadsettings"
)

func tuneScheduler(scheduler *scheduler, change func(settings *offloadsettings.Settings)) {
	settings := scheduler.currentSettings()
	change(&settings)
	scheduler.applySettings(settings)
}

func TestDeeperBackendDepthAdmitsWaitingWorkAtOnce(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")
	queue := service.scheduler.imageQueue
	entries := []*offloadEntry{
		enqueueNativeOnIdleNode(queue, "a", 10),
		enqueueNativeOnIdleNode(queue, "a", 10),
		enqueueNativeOnIdleNode(queue, "a", 10),
	}
	if outcome := admittedWithin(entries[2], 20*time.Millisecond); outcome {
		t.Fatal("third request admitted under the default depth of 2")
	}

	tuneScheduler(service.scheduler, func(settings *offloadsettings.Settings) { settings.BackendDepth = 3 })

	if !admittedWithin(entries[2], time.Second) {
		t.Fatal("waiting request stayed held after the depth was raised")
	}
}

func TestShorterRefreshIntervalTakesEffectWithoutRestart(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")
	tuneScheduler(service.scheduler, func(settings *offloadsettings.Settings) { settings.RefreshInterval = time.Hour })
	service.StartSchedulingRefresh(context.Background())
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	waitForPublishedCosts(t, service)
	firstFit := service.scheduler.publishedCosts()

	tuneScheduler(service.scheduler, func(settings *offloadsettings.Settings) { settings.RefreshInterval = time.Millisecond })

	deadline := time.Now().Add(2 * time.Second)
	for service.scheduler.publishedCosts() == firstFit {
		if time.Now().After(deadline) {
			t.Fatal("refresh kept the hour-long interval after it was shortened")
		}
		time.Sleep(time.Millisecond)
	}
}

func admittedWithin(entry *offloadEntry, wait time.Duration) bool {
	select {
	case outcome := <-entry.result:
		entry.result <- outcome
		return outcome == offloadAdmitted
	case <-time.After(wait):
		return false
	}
}
