package proxy

import (
	"sort"
	"sync"
	"time"

	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/schedulingcost"
)

type offloadCandidate struct {
	NodeID            string
	ModelID           string
	ConfigFilename    string
	Section           string
	Loaded            bool
	AcceptingBorrowed bool
	IdleFor           time.Duration
	ContextCapacity   int
	PendingCount      int64
	PendingWork       schedulingcost.Work
	PendingContext    int64
	BacklogCount      int64
	BacklogWork       schedulingcost.Work
}

type offloadHelperCandidate struct {
	offloadCandidate
	LoadIfUnloaded     bool
	RestoreAfterBorrow bool
}

type lendingOwner struct {
	owner   offloadCandidate
	helpers []offloadHelperCandidate
}

type offloadLease struct {
	Lane               string    `json:"lane"`
	OwnerNodeID        string    `json:"owner_node_id"`
	OwnerModelID       string    `json:"owner_model_id"`
	HelperNodeID       string    `json:"helper_node_id"`
	HelperModelID      string    `json:"helper_model_id"`
	LoadHelperModel    bool      `json:"load_helper_model"`
	RestoreHelperModel bool      `json:"restore_helper_model"`
	HelperSlots        int       `json:"helper_slots"`
	Probe              bool      `json:"probe,omitempty"`
	ExpiresAt          time.Time `json:"expires_at"`
}

func (lease offloadLease) slots() int {
	if lease.HelperSlots < 1 {
		return 1
	}
	return lease.HelperSlots
}

func laneModelKey(lane string, modelID string) string {
	return lane + "\x00" + modelID
}

type offloadPlanPolicy struct {
	ttl       time.Duration
	probeIdle time.Duration
	trigger   string
}

type offloadPlan struct {
	leases       []offloadLease
	decisions    []offloaddecisions.Record
	nextProbeDue time.Duration
}

func (plan *offloadPlan) probeDueIn(wait time.Duration) {
	if wait > 0 && (plan.nextProbeDue == 0 || wait < plan.nextProbeDue) {
		plan.nextProbeDue = wait
	}
}

// planOffloadLeases decides which owners may lend queued work to which of the
// helpers their links name.
//
// The comparison is what the owner needs to drain alone against what the helper
// needs to hand back the first borrowed job, model load included. A load is paid
// once when the slot opens and amortises over every job that flows through it,
// which is why a deep backlog justifies a switch that a shallow one does not.
//
// One helper node takes one owner per plan.
func planOffloadLeases(lane string, owners []lendingOwner, costs *schedulingcost.Table, now time.Time, policy offloadPlanPolicy) offloadPlan {
	var plan offloadPlan
	if costs == nil {
		return plan
	}
	claimed := map[string]bool{}
	for _, candidate := range ownersWithPendingWork(owners) {
		lease, decisions, granted := planOwner(lane, candidate, costs, claimed, now, policy)
		plan.decisions = append(plan.decisions, decisions...)
		if granted {
			claimed[lease.HelperNodeID] = true
			plan.leases = append(plan.leases, lease)
			continue
		}
		plan.probeDueIn(soonestProbeFor(candidate.helpers, claimed, ownerDemandFor(candidate.owner, costs).meanContext, policy.probeIdle))
	}
	return plan
}

func planOwner(lane string, candidate lendingOwner, costs *schedulingcost.Table, claimed map[string]bool, now time.Time, policy offloadPlanPolicy) (offloadLease, []offloaddecisions.Record, bool) {
	demand := ownerDemandFor(candidate.owner, costs)
	evaluations := make([]helperEvaluation, 0, len(candidate.helpers))
	for _, helper := range candidate.helpers {
		evaluations = append(evaluations, evaluateHelper(helper, claimed, costs, demand))
	}
	chosen, probe := chooseHelper(evaluations, demand, policy.probeIdle)
	decisions := make([]offloaddecisions.Record, 0, len(evaluations))
	for index, evaluation := range evaluations {
		decisions = append(decisions, planDecision(lane, policy.trigger, demand, evaluation, index == chosen, probe))
	}
	if chosen < 0 {
		return offloadLease{}, decisions, false
	}
	helper := evaluations[chosen].helper
	return offloadLease{
		Lane:               lane,
		OwnerNodeID:        demand.owner.NodeID,
		OwnerModelID:       demand.owner.ModelID,
		HelperNodeID:       helper.NodeID,
		HelperModelID:      helper.ModelID,
		LoadHelperModel:    helper.LoadIfUnloaded,
		RestoreHelperModel: helper.RestoreAfterBorrow,
		HelperSlots:        decisions[chosen].Slots,
		Probe:              probe,
		ExpiresAt:          now.Add(policy.ttl),
	}, decisions, true
}

func ownersWithPendingWork(owners []lendingOwner) []lendingOwner {
	backlogged := make([]lendingOwner, 0, len(owners))
	for _, candidate := range owners {
		if candidate.owner.PendingCount > 0 {
			backlogged = append(backlogged, candidate)
		}
	}
	sort.Slice(backlogged, func(left, right int) bool {
		leftTotal, rightTotal := backlogMagnitude(backlogged[left].owner.BacklogWork), backlogMagnitude(backlogged[right].owner.BacklogWork)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		if backlogged[left].owner.NodeID != backlogged[right].owner.NodeID {
			return backlogged[left].owner.NodeID < backlogged[right].owner.NodeID
		}
		return backlogged[left].owner.ModelID < backlogged[right].owner.ModelID
	})
	return backlogged
}

func backlogMagnitude(work schedulingcost.Work) float64 {
	total := 0.0
	for index := 0; index < work.Arity(); index++ {
		total += work.Term(index)
	}
	return total
}

func offloadModelKey(candidate offloadCandidate) schedulingcost.ModelKey {
	return schedulingcost.ModelKey{
		NodeID:  candidate.NodeID,
		ModelID: candidate.ModelID,
		Section: candidate.Section,
	}
}

// offloadLeaseBook is the master's record of which leases are live. A lease is
// dropped rather than revoked when it stops paying off: expiry means a helper
// that goes busy, or a master that stops polling, ends the arrangement without a
// message having to arrive.
type offloadLeaseBook struct {
	mu     sync.RWMutex
	leases map[string]offloadLease
}

func newOffloadLeaseBook() *offloadLeaseBook {
	return &offloadLeaseBook{leases: map[string]offloadLease{}}
}

func offloadLeaseBookKey(lane string, ownerNodeID string, ownerModelID string) string {
	return lane + "\x00" + ownerNodeID + "\x00" + ownerModelID
}

// Replace installs the leases this cycle planned. Anything not replanned simply
// disappears: a lease the rule no longer justifies should stop being renewed, and
// the owner stops offloading once its copy expires or the master answers without it.
func (book *offloadLeaseBook) Replace(planned []offloadLease) {
	live := make(map[string]offloadLease, len(planned))
	for _, lease := range planned {
		live[offloadLeaseBookKey(lease.Lane, lease.OwnerNodeID, lease.OwnerModelID)] = lease
	}
	book.mu.Lock()
	defer book.mu.Unlock()
	book.leases = live
}

func (book *offloadLeaseBook) Lease(lane string, ownerNodeID string, ownerModelID string, now time.Time) (offloadLease, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()
	lease, ok := book.leases[offloadLeaseBookKey(lane, ownerNodeID, ownerModelID)]
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
