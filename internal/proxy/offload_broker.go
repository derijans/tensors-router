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

type offloadLease struct {
	Lane         string    `json:"lane"`
	GroupID      string    `json:"group_id"`
	OwnerNodeID  string    `json:"owner_node_id"`
	HelperNodeID string    `json:"helper_node_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func backlogKey(lane string, groupID string) string {
	return lane + "\x00" + groupID
}

// planOffloadLeases decides which owners may lend work to which helpers.
//
// The comparison is what the owner needs to drain alone against what the helper
// needs to hand back the first borrowed job, model load included. A load is paid
// once when the slot opens and amortises over every job that flows through it,
// which is why a deep backlog justifies a switch that a shallow one does not.
//
// The group is skipped entirely unless every member is qualified. A node with too
// little history is never scheduled on a guess, and refusing the whole group until
// all of them qualify keeps the existing rotation running, which is what lets the
// unqualified members accumulate the history they need.
func planOffloadLeases(lane string, groupID string, candidates []offloadCandidate, costs *schedulingcost.Table, now time.Time, ttl time.Duration) []offloadLease {
	if len(candidates) < 2 || costs == nil {
		return nil
	}
	for _, candidate := range candidates {
		if _, qualified := costs.Estimate(offloadModelKey(candidate)); !qualified {
			return nil
		}
	}

	owners, helpers := splitOffloadRoles(candidates)
	if len(owners) == 0 || len(helpers) == 0 {
		return nil
	}

	claimed := map[string]bool{}
	leases := make([]offloadLease, 0, len(owners))
	for _, owner := range owners {
		keepMS, ok := costs.PredictQueueMS(offloadModelKey(owner), owner.BacklogCount, owner.BacklogWork)
		if !ok {
			continue
		}
		meanWork := owner.PendingWork.Mean(owner.PendingCount)
		meanContext := int(owner.PendingContext / owner.PendingCount)
		helper, helpMS, found := bestOffloadHelper(helpers, claimed, costs, meanWork, meanContext)
		if !found || helpMS >= keepMS {
			continue
		}
		claimed[helper.NodeID] = true
		leases = append(leases, offloadLease{
			Lane:         lane,
			GroupID:      groupID,
			OwnerNodeID:  owner.NodeID,
			HelperNodeID: helper.NodeID,
			ExpiresAt:    now.Add(ttl),
		})
	}
	return leases
}

func splitOffloadRoles(candidates []offloadCandidate) (owners []offloadCandidate, helpers []offloadCandidate) {
	for _, candidate := range candidates {
		switch {
		case candidate.PendingCount > 0:
			owners = append(owners, candidate)
		case candidate.AcceptingBorrowed:
			helpers = append(helpers, candidate)
		}
	}
	sort.Slice(owners, func(left, right int) bool {
		leftTotal, rightTotal := backlogMagnitude(owners[left].BacklogWork), backlogMagnitude(owners[right].BacklogWork)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return owners[left].NodeID < owners[right].NodeID
	})
	sort.Slice(helpers, func(left, right int) bool {
		return helpers[left].NodeID < helpers[right].NodeID
	})
	return owners, helpers
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

func bestOffloadHelper(helpers []offloadCandidate, claimed map[string]bool, costs *schedulingcost.Table, meanWork schedulingcost.Work, meanContext int) (offloadCandidate, float64, bool) {
	var best offloadCandidate
	bestMS := 0.0
	found := false
	for _, helper := range helpers {
		if claimed[helper.NodeID] {
			continue
		}
		if !helperWindowHolds(helper, meanContext) {
			continue
		}
		switchMS, ok := offloadSwitchMS(helper, costs)
		if !ok {
			continue
		}
		serviceMS, ok := costs.PredictMS(offloadModelKey(helper), meanWork)
		if !ok {
			continue
		}
		totalMS := switchMS + serviceMS
		if !found || totalMS < bestMS {
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

func offloadLeaseBookKey(lane string, groupID string, ownerNodeID string) string {
	return lane + "\x00" + groupID + "\x00" + ownerNodeID
}

// Replace installs the leases this cycle planned. Anything not replanned simply
// disappears: a lease the rule no longer justifies should stop being renewed, and
// the owner stops offloading as soon as its copy expires.
func (book *offloadLeaseBook) Replace(planned []offloadLease) {
	live := make(map[string]offloadLease, len(planned))
	for _, lease := range planned {
		live[offloadLeaseBookKey(lease.Lane, lease.GroupID, lease.OwnerNodeID)] = lease
	}
	book.leases = live
}

func (book *offloadLeaseBook) Lease(lane string, groupID string, ownerNodeID string, now time.Time) (offloadLease, bool) {
	lease, ok := book.leases[offloadLeaseBookKey(lane, groupID, ownerNodeID)]
	if !ok || !lease.ExpiresAt.After(now) {
		return offloadLease{}, false
	}
	return lease, true
}
