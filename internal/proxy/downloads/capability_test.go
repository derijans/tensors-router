package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/hardware"
	"tensors-router/internal/siteapi"
)

type downloadHardwareSource struct{}

func (downloadHardwareSource) Info(context.Context) hardware.Info { return hardware.Info{} }

type localNodeDeps struct{}

func (localNodeDeps) SiteControlAllowed() bool       { return true }
func (localNodeDeps) RemoteInventoryURLs() []string  { return nil }
func (localNodeDeps) NodeURLByID(string) string      { return "" }
func (localNodeDeps) ClusterClient() *cluster.Client { return nil }
func (localNodeDeps) ClusterRole() string            { return cluster.RoleStandalone }
func (localNodeDeps) NodeID() string                 { return "local" }
func (localNodeDeps) NodeURL() string                { return "" }
func (localNodeDeps) Hardware() hardware.Source      { return downloadHardwareSource{} }
func (localNodeDeps) BackendMode() string            { return backendmode.Kobold }

func TestDownloadCapabilityResponsePreservesStartupStatus(t *testing.T) {
	config := downloader.DefaultConfig(filepath.Join(t.TempDir(), "downloader.yaml"))
	config.Logging.Mode = "off"
	manager, err := downloader.NewManager(config, "hf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	handlers := New(localNodeDeps{}, manager, downloader.Capability{Enabled: true, Present: true, Working: true})
	recorder := httptest.NewRecorder()
	handlers.NodeCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/node/site/download/capabilities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response siteapi.DownloadCapability
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Capability.Enabled || !response.Capability.Present || !response.Capability.Working || !response.Capability.Available || !response.Capability.Configured {
		t.Fatalf("startup capability fields were not preserved %#v", response.Capability)
	}
}

func TestDownloadCapabilityResponsePreservesFailureDetails(t *testing.T) {
	handlers := New(localNodeDeps{}, nil, downloader.Capability{
		Enabled: true,
		Present: true,
		Working: false,
		Reason:  "load downloader configuration: downloader.yaml is invalid",
		Error:   "load downloader configuration: downloader.yaml is invalid",
	})
	recorder := httptest.NewRecorder()
	handlers.NodeCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/node/site/download/capabilities", nil))
	var response siteapi.DownloadCapability
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Capability.Enabled || !response.Capability.Present || response.Capability.Working || response.Capability.Reason == "" || response.Capability.Error != response.Capability.Reason {
		t.Fatalf("failure details were not preserved %#v", response.Capability)
	}
}
