package loaderrors

import "time"

type Phase string
type Severity string

const (
	PhaseConfigParse     Phase = "config_parse"
	PhaseAssetResolve    Phase = "asset_resolve"
	PhasePortBind        Phase = "port_bind"
	PhaseProcessSpawn    Phase = "process_spawn"
	PhaseHealthWait      Phase = "health_wait"
	PhaseReadinessWatch  Phase = "readiness_watch"
	PhaseCaptureWrite    Phase = "capture_write"
	PhasePreload         Phase = "preload"
	PhaseUnload          Phase = "unload"
	PhaseSeparateRuntime Phase = "separate_runtime"
	PhaseStartup         Phase = "startup"
	PhaseDownload        Phase = "download"

	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type RecordInput struct {
	Phase       Phase
	Severity    Severity
	Source      string
	NodeID      string
	ModelID     string
	ConfigName  string
	Backend     string
	BackendMode string
	Message     string
	ExitError   string
	Output      string
	Truncated   bool
	Secrets     []string
}

type Record struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	Occurrences int64     `json:"occurrences"`
	NodeID      string    `json:"node_id,omitempty"`
	ModelID     string    `json:"model_id,omitempty"`
	ConfigName  string    `json:"config_name,omitempty"`
	Backend     string    `json:"backend,omitempty"`
	BackendMode string    `json:"backend_mode,omitempty"`
	Phase       Phase     `json:"phase"`
	Severity    Severity  `json:"severity"`
	Source      string    `json:"source,omitempty"`
	Message     string    `json:"message"`
	ExitError   string    `json:"exit_error,omitempty"`
	Output      string    `json:"output,omitempty"`
	Truncated   bool      `json:"truncated,omitempty"`
}

type ListFilter struct {
	NodeID   string
	Phase    Phase
	Severity Severity
	Limit    int
}

type ListResult struct {
	Records []Record `json:"records"`
}
