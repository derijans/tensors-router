package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestCloseStopsTheSchedulingRefresh(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")
	service.scheduler.refreshInterval = time.Millisecond
	service.StartSchedulingRefresh(context.Background())
	waitForPublishedCosts(t, service)

	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	costsAtClose := service.scheduler.publishedCosts()
	time.Sleep(20 * service.scheduler.refreshInterval)

	if service.scheduler.publishedCosts() != costsAtClose {
		t.Fatal("scheduling refresh republished costs after Close returned")
	}
}

func waitForPublishedCosts(t *testing.T, service *Service) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for service.scheduler.publishedCosts() == nil {
		if time.Now().After(deadline) {
			t.Fatal("scheduling refresh never published costs")
		}
		time.Sleep(time.Millisecond)
	}
}
