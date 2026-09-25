package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/routinggroups"
)

const (
	ownerImageID  = "krea-dream"
	helperImageID = "krea11-dream"
)

type lendingCluster struct {
	master          *Service
	slave           *Service
	masterDecisions *recordedDecisions
	slaveDecisions  *recordedDecisions
	masterServed    *atomic.Int32
	releaseSlave    chan struct{}
	slaveURL        string
}

func newLendingCluster(t *testing.T) lendingCluster {
	t.Helper()
	var masterServed atomic.Int32
	releaseSlave := make(chan struct{})
	master, _ := newTestServiceWithConfigContents(t, imageBackendHandler(func() { masterServed.Add(1) }), map[string]string{
		"krea11": `{"nomodel":true,"sdmodel":"dream.safetensors"}`,
	})
	slave, _ := newTestServiceWithConfigContents(t, imageBackendHandler(func() { <-releaseSlave }), map[string]string{
		"krea": `{"nomodel":true,"sdmodel":"dream.safetensors"}`,
	})
	masterServer := httptest.NewServer(master)
	slaveServer := httptest.NewServer(slave)
	t.Cleanup(masterServer.Close)
	t.Cleanup(slaveServer.Close)

	joinCluster(t, master, "master", cluster.RoleMaster, masterServer.URL, masterServer.URL, slaveServer.URL)
	joinCluster(t, slave, "slave", cluster.RoleSlave, slaveServer.URL, masterServer.URL, masterServer.URL)
	master.masterURL = ""

	helperModel := testClusterImageModel("krea11", "master", "weights", "config-master", cluster.SourceMaster, "dream")
	helperModel.NodeURL = masterServer.URL
	ownerModel := testClusterImageModel("krea", "slave", "weights", "config-slave", cluster.SourceLocal, "dream")
	ownerModel.NodeURL = slaveServer.URL
	master.registry = registryWith(t, cluster.RoleMaster, "master", masterServer.URL, helperModel)
	if err := master.registry.UpdateNode(cluster.Snapshot{NodeID: "slave", NodeURL: slaveServer.URL, Models: []cluster.Model{ownerModel}, ProtocolVersion: cluster.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	slave.registry = registryWith(t, cluster.RoleSlave, "slave", slaveServer.URL, ownerModel)

	link := routingLinkSnapshot{Image: []routinggroups.Link{{
		Owner:          routinggroups.Endpoint{NodeID: "slave", ModelID: ownerImageID},
		Helper:         routinggroups.Endpoint{NodeID: "master", ModelID: helperImageID},
		LoadIfUnloaded: true,
	}}}
	master.installRoutingLinks(link)
	slave.installRoutingLinks(link)

	master.scheduler.probeIdle = time.Nanosecond
	masterDecisions, slaveDecisions := &recordedDecisions{}, &recordedDecisions{}
	master.scheduler.decisions = masterDecisions
	slave.scheduler.decisions = slaveDecisions
	t.Cleanup(func() {
		select {
		case <-releaseSlave:
		default:
			close(releaseSlave)
		}
	})
	return lendingCluster{
		master:          master,
		slave:           slave,
		masterDecisions: masterDecisions,
		slaveDecisions:  slaveDecisions,
		masterServed:    &masterServed,
		releaseSlave:    releaseSlave,
		slaveURL:        slaveServer.URL,
	}
}

func imageBackendHandler(onGenerate func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sdapi/v1/txt2img" {
			onGenerate()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":["x"]}`))
	})
}

func joinCluster(t *testing.T, service *Service, nodeID string, role string, nodeURL string, masterURL string, peers ...string) {
	t.Helper()
	service.nodeID = nodeID
	service.clusterRole = role
	service.nodeURL = nodeURL
	service.masterURL = masterURL
	service.clusterToken = "secret"
	service.clusterClient = cluster.NewClient("secret")
	if err := service.clusterClient.AllowBaseURLs(append([]string{nodeURL}, peers...)...); err != nil {
		t.Fatal(err)
	}
}

func registryWith(t *testing.T, role string, nodeID string, nodeURL string, local cluster.Model) *cluster.Registry {
	t.Helper()
	registry := cluster.NewRegistry(role, nodeID, nodeURL)
	if err := registry.UpdateLocal([]cluster.Model{local}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func postOwnerImage(t *testing.T, slaveURL string) int {
	t.Helper()
	body := `{"sd_model_checkpoint":"` + ownerImageID + `","width":512,"height":512,"steps":8}`
	response, err := http.Post(slaveURL+"/sdapi/v1/txt2img", "application/json", strings.NewReader(body))
	if err != nil {
		t.Error(err)
		return 0
	}
	defer response.Body.Close()
	return response.StatusCode
}

func TestSlaveLendsHeldImageWorkToAnIdleMasterThatDecidesOnEveryEvent(t *testing.T) {
	lending := newLendingCluster(t)
	const requests = 4
	codes := make(chan int, requests)
	for range requests {
		go func() { codes <- postOwnerImage(t, lending.slaveURL) }()
	}

	lending.waitForMasterToServe(t, 2)
	close(lending.releaseSlave)
	for range requests {
		if code := <-codes; code != http.StatusOK {
			t.Fatalf("owner request status %d, want 200 whether served locally or lent", code)
		}
	}

	if lent := lending.slaveDecisions.withOutcome(offloaddecisions.OutcomeLent); len(lent) == 0 || lent[0].HelperNodeID != "master" {
		t.Fatalf("slave lent decisions = %+v, want held work lent to the master", lent)
	}
	probes := lending.masterDecisions.withOutcome(offloaddecisions.OutcomeProbe)
	if len(probes) == 0 || probes[0].OwnerNodeID != "slave" || probes[0].Reason != reasonOwnerUnpriced {
		t.Fatalf("master probe decisions = %+v, want the unpriced pair probed", probes)
	}
	if probes[0].Trigger != queueEventEnqueued {
		t.Fatalf("first probe trigger = %q, want the owner's enqueue event to have driven it", probes[0].Trigger)
	}
	if !anyTrigger(probes, queueEventBorrowedCompleted) && !anyTrigger(probes, planTriggerProbeDue) {
		t.Fatalf("master probe decisions = %+v, want the second lend decided when the first lent result came back", probes)
	}
}

func (lending lendingCluster) waitForMasterToServe(t *testing.T, want int32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if lending.masterServed.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	lending.masterDecisions.mu.Lock()
	lending.slaveDecisions.mu.Lock()
	defer lending.masterDecisions.mu.Unlock()
	defer lending.slaveDecisions.mu.Unlock()
	t.Fatalf("master served %d lent requests, want %d\nmaster decisions: %+v\nslave decisions: %+v",
		lending.masterServed.Load(), want, lending.masterDecisions.records, lending.slaveDecisions.records)
}

func anyTrigger(records []offloaddecisions.Record, trigger string) bool {
	for _, record := range records {
		if record.Trigger == trigger {
			return true
		}
	}
	return false
}
