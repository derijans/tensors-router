package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloadsettings"
	"tensors-router/internal/routerstore/routerstoretest"
	"tensors-router/internal/siteapi"
)

func withStoredLendingSettings(t *testing.T, service *Service, fileValues offloadsettings.Values) {
	t.Helper()
	handle := routerstoretest.Open(t, offloadsettings.SchemaModule{})
	service.lending = newLendingSettings(fileValues, offloadsettings.NewStore(handle.DB(), handle.Reader()), service.applyLendingSettings)
	service.reloadLendingSettings(t.Context())
}

func sendLendingSettings(t *testing.T, service *Service, method string, target string, body string) (int, siteapi.LendingSettingsResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
	var response siteapi.LendingSettingsResponse
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	return recorder.Code, response
}

func lendingEntry(t *testing.T, response siteapi.LendingSettingsResponse, key string) offloadsettings.Entry {
	t.Helper()
	for _, entry := range response.Entries {
		if entry.Key == key {
			return entry
		}
	}
	t.Fatalf("no %s entry in %+v", key, response.Entries)
	return offloadsettings.Entry{}
}

func TestSiteLendingSettingsOverrideAppliesAndDefaultButtonFallsBack(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	withStoredLendingSettings(t, service, offloadsettings.Values{"offload_probe_idle": "10s"})
	const path = "/router/v1/site/offload/settings"

	code, response := sendLendingSettings(t, service, http.MethodPost, path, `{"values":{"offload_probe_idle":"2s"}}`)
	if entry := lendingEntry(t, response, "offload_probe_idle"); code != http.StatusOK || entry.Source != offloadsettings.SourceDatabase || entry.Effective != "2s" {
		t.Fatalf("after override status=%d entry=%+v", code, entry)
	}
	if probeIdle := service.scheduler.currentSettings().ProbeIdle; probeIdle != 2*time.Second {
		t.Fatalf("scheduler probe idle = %v, want the override applied at once", probeIdle)
	}

	code, response = sendLendingSettings(t, service, http.MethodDelete, path+"?key=offload_probe_idle", "")
	if entry := lendingEntry(t, response, "offload_probe_idle"); code != http.StatusOK || entry.Source != offloadsettings.SourceConfig || entry.Effective != "10s" {
		t.Fatalf("after reset status=%d entry=%+v, want the config value back", code, entry)
	}
	if probeIdle := service.scheduler.currentSettings().ProbeIdle; probeIdle != 10*time.Second {
		t.Fatalf("scheduler probe idle = %v, want the config value after reset", probeIdle)
	}
}

func TestSiteLendingSettingsRejectsAnInvalidValueWithoutApplyingAny(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	withStoredLendingSettings(t, service, nil)
	before := service.scheduler.currentSettings()

	code, _ := sendLendingSettings(t, service, http.MethodPost, "/router/v1/site/offload/settings", `{"values":{"offload_probe_idle":"2s","scheduling_backend_depth":"0"}}`)

	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if service.scheduler.currentSettings() != before {
		t.Fatal("a rejected batch changed the running settings")
	}
}

func TestMasterPushesItsLendingSettingsToSlaves(t *testing.T) {
	slave, _ := newTestService(t, http.NotFoundHandler())
	slaveServer := httptest.NewServer(slave)
	t.Cleanup(slaveServer.Close)
	master, _ := newTestService(t, http.NotFoundHandler())
	masterServer := httptest.NewServer(master)
	t.Cleanup(masterServer.Close)
	joinCluster(t, master, "master", cluster.RoleMaster, masterServer.URL, "", slaveServer.URL)
	joinCluster(t, slave, "slave", cluster.RoleSlave, slaveServer.URL, masterServer.URL, masterServer.URL)
	master.registry = cluster.NewRegistry(cluster.RoleMaster, "master", masterServer.URL)
	if err := master.registry.UpdateNode(cluster.Snapshot{NodeID: "slave", NodeURL: slaveServer.URL, ProtocolVersion: cluster.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	withStoredLendingSettings(t, master, nil)

	if code, _ := sendLendingSettings(t, master, http.MethodPost, "/router/v1/site/offload/settings", `{"values":{"scheduling_min_samples":"5"}}`); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}

	if got, want := slave.scheduler.currentSettings(), master.scheduler.currentSettings(); got != want || got.MinSamples != 5 {
		t.Fatalf("slave runs %+v, want the master's %+v", got, want)
	}
}
