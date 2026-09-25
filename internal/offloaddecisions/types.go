package offloaddecisions

import "time"

type Kind string

const (
	KindPlan     Kind = "plan"
	KindDispatch Kind = "dispatch"
	KindHelper   Kind = "helper"
)

const (
	OutcomeGranted = "granted"
	OutcomeProbe   = "probe"
	OutcomeSkipped = "skipped"

	OutcomeLent         = "lent"
	OutcomeReturned     = "returned"
	OutcomeLeaseUpdated = "lease_updated"
	OutcomeLeaseCleared = "lease_cleared"

	OutcomeNativeWaitedBehindBorrowed = "native_waited_behind_borrowed"
	OutcomeBorrowedReturned           = "borrowed_returned"
)

const (
	ServiceSourceHelper        = "helper"
	ServiceSourceOwnerFallback = "owner_fallback"
)

type Record struct {
	RecordedAt    time.Time `json:"recorded_at"`
	NodeID        string    `json:"node_id"`
	Kind          Kind      `json:"kind"`
	Trigger       string    `json:"trigger,omitempty"`
	Lane          string    `json:"lane"`
	OwnerNodeID   string    `json:"owner_node_id,omitempty"`
	OwnerModelID  string    `json:"owner_model_id,omitempty"`
	HelperNodeID  string    `json:"helper_node_id,omitempty"`
	HelperModelID string    `json:"helper_model_id,omitempty"`
	Outcome       string    `json:"outcome"`
	Reason        string    `json:"reason,omitempty"`
	PendingCount  int64     `json:"pending_count,omitempty"`
	BacklogCount  int64     `json:"backlog_count,omitempty"`
	KeepMS        float64   `json:"keep_ms,omitempty"`
	SwitchMS      float64   `json:"switch_ms,omitempty"`
	ServiceMS     float64   `json:"service_ms,omitempty"`
	OwnerJobMS    float64   `json:"owner_job_ms,omitempty"`
	ServiceSource string    `json:"service_source,omitempty"`
	HelperIdleMS  int64     `json:"helper_idle_ms,omitempty"`
	Slots         int       `json:"slots,omitempty"`
	LentOut       int       `json:"lent_out,omitempty"`
	BorrowedAhead int       `json:"borrowed_ahead,omitempty"`
	WaitMS        int64     `json:"wait_ms,omitempty"`
}
