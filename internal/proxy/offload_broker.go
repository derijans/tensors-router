package proxy

import (
	"sort"
	"time"

	"tensors-router/internal/schedulingcost"
)

type offloadCandidate struct {
	NodeID            string
	ModelID           string
	ConfigFilename    string
	Section           string
	Loaded            bool
	AcceptingBorrowed bool
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
	ExpiresAt          time.Time `json:"expires_at"`
}

func laneModelKey(lane string, modelID string) string {
	return lane + "\x00" + modelID
}

// planOffloadLeases decides which owners may lend queued work to which of the
// helpers their links name.
//
// The comparison is what the owner needs to drain alone against what the helper
// needs to hand back the first borrowed job, model load included. A load is paid
// once when the slot opens and amortises over every job that flows through it,
// which is why a deep backlog justifies a switch that a shallow one does not.
//
// A node with too little history is never scheduled on a guess: an owner or helper
// the cost table cannot price is skipped. One helper node takes one owner per plan.
func planOffloadLeases(lane string, owners []lendingOwner, costs *schedulingcost.Table, now time.Time, ttl time.Duration) []offloadLease {
	if costs == nil {
		return nil
	}
	backlogged := ownersWithPendingWork(owners)
	claimed := map[string]bool{}
	leases := make([]offloadLease, 0, len(backlogged))
	for _, candidate := range backlogged {
		owner := candidate.owner
		keepMS, ok := costs.PredictQueueMS(offloadModelKey(owner), owner.BacklogCount, owner.BacklogWork)
		if !ok {
			continue
		}
		meanWork := owner.PendingWork.Mean(owner.PendingCount)
		meanContext := int(owner.PendingContext / owner.PendingCount)
		helper, helpMS, found := bestOffloadHelper(candidate.helpers, claimed, costs, meanWork, meanContext)
		if !found || helpMS >= keepMS {
			continue
		}
		claimed[helper.NodeID] = true
		leases = append(leases, offloadLease{
			Lane:               lane,
			OwnerNodeID:        owner.NodeID,
			OwnerModelID:       owner.ModelID,
			HelperNodeID:       helper.NodeID,
			HelperModelID:      helper.ModelID,
			LoadHelperModel:    helper.LoadIfUnloaded,
			RestoreHelperModel: helper.RestoreAfterBorrow,
			ExpiresAt:          now.Add(ttl),
		})
	}
	return leases
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

func helperWindowHolds(helper offloadCandidate, meanContext int) bool {
	return meanContext <= 0 || helper.ContextCapacity >= meanContext
}

func helperMayServe(helper offloadHelperCandidate, claimed map[string]bool, meanContext int) bool {
	return helper.AcceptingBorrowed &&
		!claimed[helper.NodeID] &&
		(helper.Loaded || helper.LoadIfUnloaded) &&
		helperWindowHolds(helper.offloadCandidate, meanContext)
}

func bestOffloadHelper(helpers []offloadHelperCandidate, claimed map[string]bool, costs *schedulingcost.Table, meanWork schedulingcost.Work, meanContext int) (offloadHelperCandidate, float64, bool) {
	var best offloadHelperCandidate
	bestMS := 0.0
	found := false
	for _, helper := range helpers {
		if !helperMayServe(helper, claimed, meanContext) {
			continue
		}
		switchMS, ok := offloadSwitchMS(helper.offloadCandidate, costs)
		if !ok {
			continue
		}
		serviceMS, ok := costs.PredictMS(offloadModelKey(helper.offloadCandidate), meanWork)
		if !ok {
			continue
		}
		totalMS := switchMS + serviceMS
		if !found || totalMS < bestMS || (totalMS == bestMS && helper.NodeID < best.NodeID) {
			best, bestMS, found = helper, totalMS, true
		}
	}
	return best, bestMS, found
}

func offloadSwitchMS(helper offloadCandidate, costs *schedulingcost.Table) (float64, bool) {
	if helper.Loaded {
		return 0, true
	}
	return costs.LoadMS(schedulingcost.LoadKey{NodeID: helper.NodeID, ConfigFilename: helper.ConfigFilename})
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
// the owner stops offloading as soon as its copy expires.
func (book *offloadLeaseBook) Replace(planned []offloadLease) {
	live := make(map[string]offloadLease, len(planned))
	for _, lease := range planned {
		live[offloadLeaseBookKey(lease.Lane, lease.OwnerNodeID, lease.OwnerModelID)] = lease
	}
	book.leases = live
}

func (book *offloadLeaseBook) Lease(lane string, ownerNodeID string, ownerModelID string, now time.Time) (offloadLease, bool) {
	lease, ok := book.leases[offloadLeaseBookKey(lane, ownerNodeID, ownerModelID)]
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
