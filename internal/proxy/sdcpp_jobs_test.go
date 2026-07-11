package proxy

import (
	"testing"
	"time"
)

func TestSdcppJobRoutesExpire(t *testing.T) {
	store := newSdcppJobStore()
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	store.remember("job-a", sdcppJobTarget{publicImageID: "image-a"})
	if target, ok := store.routeForPath("/sdcpp/v1/jobs/job-a"); !ok || target.publicImageID != "image-a" {
		t.Fatalf("unexpected remembered route %#v ok=%t", target, ok)
	}
	now = now.Add(sdcppJobLifetime)
	if _, ok := store.routeForPath("/sdcpp/v1/jobs/job-a"); ok {
		t.Fatal("expired job route remained available")
	}
}
