package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const slowRemoteWork = 250 * time.Millisecond

func slowNodeServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(slowRemoteWork):
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// A first load on a portable config transfers its assets from a peer before the
// backend starts, which routinely outlasts cluster.control_timeout.
func TestLoadOutlastsControlTimeout(t *testing.T) {
	server := slowNodeServer(t)
	client := NewClientWithTimeout("secret", 25*time.Millisecond, server.URL)

	if err := client.Load(context.Background(), server.URL, "portable-model"); err != nil {
		t.Fatalf("load was cut off by the control timeout: %v", err)
	}
}

func TestUnloadOutlastsControlTimeout(t *testing.T) {
	server := slowNodeServer(t)
	client := NewClientWithTimeout("secret", 25*time.Millisecond, server.URL)

	if err := client.Unload(context.Background(), server.URL, "portable-model", ""); err != nil {
		t.Fatalf("unload was cut off by the control timeout: %v", err)
	}
}

func TestLoadStillHonoursContextDeadline(t *testing.T) {
	server := slowNodeServer(t)
	client := NewClientWithTimeout("secret", time.Minute, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	if err := client.Load(ctx, server.URL, "portable-model"); err == nil {
		t.Fatal("expected the caller deadline to bound the load")
	}
}

func TestControlRequestsKeepControlTimeout(t *testing.T) {
	server := slowNodeServer(t)
	client := NewClientWithTimeout("secret", 25*time.Millisecond, server.URL)

	if _, err := client.FetchSnapshot(context.Background(), server.URL); err == nil {
		t.Fatal("expected the control timeout to bound a snapshot fetch")
	}
}
