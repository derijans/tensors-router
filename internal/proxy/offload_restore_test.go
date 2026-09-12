package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func borrowedRestoreContext() context.Context {
	ctx := context.WithValue(context.Background(), offloadContextKey{}, true)
	return context.WithValue(ctx, offloadRestoreContextKey{}, true)
}

func borrowedContext() context.Context {
	return context.WithValue(context.Background(), offloadContextKey{}, true)
}

func newBorrowRestoreTestService(t *testing.T) *Service {
	t.Helper()
	service, _ := newTestServiceWithModels(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}), "a", "b")
	service.offloadRestoreDelay = 20 * time.Millisecond
	return service
}

func waitForLoadedModel(t *testing.T, service *Service, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, filename := service.textRuntime.state.loadedModel(); filename == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, filename := service.textRuntime.state.loadedModel()
	t.Fatalf("loaded model = %q, want %q", filename, want)
}

func TestBorrowRestoreReloadsTheDisplacedModel(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()

	if err := service.loadLocalConfig(context.Background(), mode, "a", "a.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	if err := service.loadLocalConfig(borrowedRestoreContext(), mode, "b", "b.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	if _, filename := service.textRuntime.state.loadedModel(); filename != "b.kcpps" {
		t.Fatalf("loaded model = %q, want b.kcpps immediately after the borrowed acquire", filename)
	}

	waitForLoadedModel(t, service, "a.kcpps")
}

func TestBorrowRestoreDoesNothingWithoutTheRestoreMarker(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()

	if err := service.loadLocalConfig(context.Background(), mode, "a", "a.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	if err := service.loadLocalConfig(borrowedContext(), mode, "b", "b.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * service.offloadRestoreDelay)
	if _, filename := service.textRuntime.state.loadedModel(); filename != "b.kcpps" {
		t.Fatalf("loaded model = %q, want b.kcpps to stay loaded with no restore requested", filename)
	}
}

func TestBorrowRestoreIsCancelledByNativeTraffic(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()

	if err := service.loadLocalConfig(context.Background(), mode, "a", "a.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	if err := service.loadLocalConfig(borrowedRestoreContext(), mode, "b", "b.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	if err := service.loadLocalConfig(context.Background(), mode, "b", "b.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * service.offloadRestoreDelay)
	if _, filename := service.textRuntime.state.loadedModel(); filename != "b.kcpps" {
		t.Fatalf("loaded model = %q, want b.kcpps to stay loaded once native traffic claimed it", filename)
	}
}

func TestBorrowRestoreDoesNotFireWhileBorrowedWorkIsInFlight(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()

	if err := service.loadLocalConfig(context.Background(), mode, "a", "a.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	if err := service.loadLocalConfig(borrowedRestoreContext(), mode, "b", "b.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}

	entry := service.textQueue.Enqueue(queuedRequest{groupID: "group", origin: borrowedFromPeer}, nodeActivity(true), time.Now())
	if _, err := service.textQueue.Await(context.Background(), entry); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * service.offloadRestoreDelay)
	if _, filename := service.textRuntime.state.loadedModel(); filename != "b.kcpps" {
		t.Fatalf("loaded model = %q, want b.kcpps to stay loaded while borrowed work is in flight", filename)
	}

	service.textQueue.Complete(entry)
}
