package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/schedulingcost"
)

type recordedDecisions struct {
	mu      sync.Mutex
	records []offloaddecisions.Record
}

func (recorded *recordedDecisions) Record(record offloaddecisions.Record) {
	recorded.mu.Lock()
	defer recorded.mu.Unlock()
	recorded.records = append(recorded.records, record)
}

func (recorded *recordedDecisions) withOutcome(outcome string) []offloaddecisions.Record {
	recorded.mu.Lock()
	defer recorded.mu.Unlock()
	var matching []offloaddecisions.Record
	for _, record := range recorded.records {
		if record.Outcome == outcome {
			matching = append(matching, record)
		}
	}
	return matching
}

const lentModelID = "combo-dream"

func schedulerRecordingDecisions(t *testing.T) (*scheduler, *recordedDecisions) {
	t.Helper()
	service := newLinkedImageService(t, nil, true)
	recorded := &recordedDecisions{}
	service.scheduler.decisions = recorded
	return service.scheduler, recorded
}

func holdNativeImageRequests(scheduler *scheduler, count int) []*offloadEntry {
	entries := make([]*offloadEntry, 0, count)
	for range count {
		entries = append(entries, enqueueNativeOnIdleNode(scheduler.imageQueue, lentModelID, testJobWork))
	}
	return entries
}

func imageLeaseWithSlots(slots int) offloadLease {
	return offloadLease{
		Lane:          cluster.RouteLaneImage,
		OwnerNodeID:   "slave",
		OwnerModelID:  lentModelID,
		HelperNodeID:  "master",
		HelperModelID: "combo-alt-dream",
		HelperSlots:   slots,
		ExpiresAt:     time.Now().Add(time.Minute),
	}
}

func withdrawnEntries(entries []*offloadEntry) []*offloadEntry {
	var withdrawn []*offloadEntry
	for _, entry := range entries {
		select {
		case outcome := <-entry.result:
			if outcome == offloadWithdrawn {
				withdrawn = append(withdrawn, entry)
			} else {
				entry.result <- outcome
			}
		default:
		}
	}
	return withdrawn
}

func TestLeaseSlotsBoundHowManyHeldRequestsAreLent(t *testing.T) {
	scheduler, recorded := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 10)

	scheduler.acceptOffloadLease(imageLeaseWithSlots(2), queueEventEnqueued)

	if withdrawn := withdrawnEntries(entries); len(withdrawn) != 2 {
		t.Fatalf("withdrawn %d, want exactly the lease's 2 slots", len(withdrawn))
	}
	if lent := scheduler.lent.Count(cluster.RouteLaneImage, lentModelID); lent != 2 {
		t.Fatalf("lent out %d, want 2", lent)
	}
	if lentRows := recorded.withOutcome(offloaddecisions.OutcomeLent); len(lentRows) != 2 {
		t.Fatalf("lent decisions = %+v, want one per lent request", lentRows)
	}
}

func TestFinishedLentRequestWaitsForTheMastersNextAnswerBeforeLendingMore(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 10)
	scheduler.acceptOffloadLease(imageLeaseWithSlots(2), queueEventEnqueued)
	lent := withdrawnEntries(entries)

	scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, lent[0], false)
	if early := withdrawnEntries(entries); len(early) != 0 {
		t.Fatalf("withdrawn %d on the owner's own initiative, want the master to decide first", len(early))
	}
	scheduler.acceptOffloadLease(imageLeaseWithSlots(2), queueEventBorrowedCompleted)

	if next := withdrawnEntries(entries); len(next) != 1 {
		t.Fatalf("withdrawn %d after the master's answer, want the freed slot refilled", len(next))
	}
	if count := scheduler.lent.Count(cluster.RouteLaneImage, lentModelID); count != 2 {
		t.Fatalf("lent out %d, want the slots topped back up to 2", count)
	}
}

func TestNewHeldRequestIsNotLentBeforeTheMasterAnswers(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	holdNativeImageRequests(scheduler, 2)
	scheduler.offloadLeases.Store(laneModelKey(cluster.RouteLaneImage, lentModelID), imageLeaseWithSlots(2))

	entries := holdNativeImageRequests(scheduler, 1)

	if withdrawn := withdrawnEntries(entries); len(withdrawn) != 0 {
		t.Fatalf("withdrawn %d on enqueue, want lending to wait for the master's decision", len(withdrawn))
	}
}

func TestShrunkLeaseStopsLendingWithoutRecallingWhatIsOut(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 10)
	scheduler.acceptOffloadLease(imageLeaseWithSlots(2), queueEventEnqueued)
	lent := withdrawnEntries(entries)

	scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventCompleted)
	scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, lent[0], false)

	if next := withdrawnEntries(entries); len(next) != 0 {
		t.Fatalf("withdrawn %d, want none while the one remaining lent request fills the single slot", len(next))
	}
	if count := scheduler.lent.Count(cluster.RouteLaneImage, lentModelID); count != 1 {
		t.Fatalf("lent out %d, want the second lent request left where it is", count)
	}
}

func TestMastersRefusalClearsTheLeaseAndStopsLending(t *testing.T) {
	scheduler, recorded := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 10)
	scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventEnqueued)
	lent := withdrawnEntries(entries)

	scheduler.applyDecidedLease(queueEvent{Lane: cluster.RouteLaneImage, ModelID: lentModelID, Trigger: queueEventCompleted}, offloadLease{}, false)
	scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, lent[0], false)

	if next := withdrawnEntries(entries); len(next) != 0 {
		t.Fatalf("withdrawn %d after the master refused, want none", len(next))
	}
	if cleared := recorded.withOutcome(offloaddecisions.OutcomeLeaseCleared); len(cleared) != 1 {
		t.Fatalf("lease cleared decisions = %+v, want one", cleared)
	}
}

func TestReturnedLentRequestIsRecordedAndFreesItsSlot(t *testing.T) {
	scheduler, recorded := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 3)
	scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventEnqueued)
	lent := withdrawnEntries(entries)

	scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, lent[0], true)

	if returned := recorded.withOutcome(offloaddecisions.OutcomeReturned); len(returned) != 1 || returned[0].HelperNodeID != "master" {
		t.Fatalf("returned decisions = %+v, want one naming the helper", returned)
	}
}

func TestOwnRequestWaitingBehindBorrowedWorkIsRecorded(t *testing.T) {
	scheduler, recorded := schedulerRecordingDecisions(t)
	first := enqueueBorrowedOnIdleNode(scheduler.imageQueue, lentModelID, testJobWork)
	second := enqueueBorrowedOnIdleNode(scheduler.imageQueue, lentModelID, testJobWork)
	mustBeAdmitted(t, first, "first borrowed")
	mustBeAdmitted(t, second, "second borrowed")

	admitted := make(chan error, 1)
	go func() {
		_, err := scheduler.enterImageQueue(context.Background(), lentModelID, schedulingcost.ImageWork(testJobWork), false)
		admitted <- err
	}()
	waitForPendingNative(t, scheduler)
	scheduler.imageQueue.Complete(first)
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}

	waited := recorded.withOutcome(offloaddecisions.OutcomeNativeWaitedBehindBorrowed)
	if len(waited) != 1 || waited[0].BorrowedAhead != 2 || waited[0].Kind != offloaddecisions.KindHelper {
		t.Fatalf("waited decisions = %+v, want one helper row with 2 borrowed jobs ahead", waited)
	}
}

func TestBorrowedRequestRefusedOnArrivalIsRecorded(t *testing.T) {
	scheduler, recorded := schedulerRecordingDecisions(t)
	enqueueNativeOnIdleNode(scheduler.imageQueue, lentModelID, testJobWork)

	admission, err := scheduler.enterImageQueue(context.Background(), lentModelID, schedulingcost.ImageWork(testJobWork), true)
	if err != nil {
		t.Fatal(err)
	}

	if admission.outcome != offloadReturned {
		t.Fatalf("outcome = %v, want returned while own work is held", admission.outcome)
	}
	returned := recorded.withOutcome(offloaddecisions.OutcomeBorrowedReturned)
	if len(returned) != 1 || returned[0].Reason != reasonHelperBusyOnArrival {
		t.Fatalf("returned decisions = %+v, want one refused on arrival", returned)
	}
}

func waitForPendingNative(t *testing.T, scheduler *scheduler) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if scheduler.imageQueue.HoldsPendingWork(lentModelID) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("own request never reached the queue")
}

func TestSlaveAppliesTheLeaseTheMasterAnswersAnEventWith(t *testing.T) {
	var received queueEvent
	answer := `{"lease":{"lane":"image","owner_node_id":"slave","owner_model_id":"combo-dream","helper_node_id":"master","helper_model_id":"combo-alt-dream","helper_slots":1,"expires_at":"` + time.Now().Add(time.Minute).Format(time.RFC3339Nano) + `"}}`
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != offloadEventPath || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(master.Close)
	service := slaveReportingTo(t, master.URL)
	entries := holdNativeImageRequests(service.scheduler, 4)

	service.scheduler.decideOnQueueEvent(context.Background(), queueEvent{Lane: cluster.RouteLaneImage, OwnerNodeID: service.nodeID, ModelID: lentModelID, Trigger: queueEventCompleted})

	if received.Trigger != queueEventCompleted || received.ModelID != lentModelID {
		t.Fatalf("master received %+v, want the completion event for the owner model", received)
	}
	if withdrawn := withdrawnEntries(entries); len(withdrawn) != 1 {
		t.Fatalf("withdrawn %d, want the one slot the master granted", len(withdrawn))
	}
}

func TestSlaveDropsItsLeaseWhenTheMasterAnswersWithout(t *testing.T) {
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(master.Close)
	service := slaveReportingTo(t, master.URL)
	service.scheduler.offloadLeases.Store(laneModelKey(cluster.RouteLaneImage, lentModelID), imageLeaseWithSlots(2))

	service.scheduler.decideOnQueueEvent(context.Background(), queueEvent{Lane: cluster.RouteLaneImage, OwnerNodeID: service.nodeID, ModelID: lentModelID, Trigger: queueEventCompleted})

	if _, live := service.scheduler.activeOffloadLease(cluster.RouteLaneImage, lentModelID, time.Now()); live {
		t.Fatal("lease survived a master answer that granted none")
	}
}

func slaveReportingTo(t *testing.T, masterURL string) *Service {
	t.Helper()
	service := newLinkedImageService(t, nil, true)
	service.clusterRole = cluster.RoleSlave
	service.masterURL = masterURL
	service.clusterClient = cluster.NewClient("secret")
	if err := service.clusterClient.AllowBaseURLs(masterURL); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestOffloadEventEndpointRequiresTheClusterToken(t *testing.T) {
	service := newLinkedImageService(t, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, offloadEventPath, strings.NewReader(`{"lane":"image","owner_node_id":"slave","model_id":"combo-dream","trigger":"completed"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401 without the cluster token", recorder.Code)
	}
}

func TestOffloadEventIsRefusedByANodeThatIsNotTheMaster(t *testing.T) {
	service := newLinkedImageService(t, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, offloadEventPath, strings.NewReader(`{"lane":"image","owner_node_id":"slave","model_id":"combo-dream","trigger":"completed"}`))
	request.Header.Set("Authorization", "Bearer secret")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status %d body %s, want 409 from a standalone node", recorder.Code, recorder.Body.String())
	}
}
