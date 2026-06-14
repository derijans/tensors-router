package benchmark

import "encoding/json"

const (
	TypeGeneral = "general"
	TypeSection = "section"

	SectionAll     = "all"
	SectionRuntime = "runtime"
	SectionLLM     = "llm"
	SectionEmbed   = "embed"
	SectionImage   = "image"
	SectionVoice   = "voice"
	SectionMusic   = "music"

	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
	StatusPartial = "partial"
	StatusRunning = "running"
)

const (
	MetricRequestMS             = "request_ms"
	MetricModelLoadMS           = "model_load_ms"
	MetricTotalRunMS            = "total_run_ms"
	MetricTotalStartMS          = "total_start_ms"
	MetricFirstCompletionMS     = "first_completion_ms"
	MetricTokensPerSecond       = "tokens_per_second"
	MetricPromptTokensPerSecond = "prompt_tokens_per_second"
	MetricCompletionTokens      = "completion_tokens"
	MetricPromptTokens          = "prompt_tokens"
	MetricSectionsTotal         = "sections_total"
	MetricSectionsSuccess       = "sections_success"
	MetricSectionsFailed        = "sections_failed"
	MetricSectionsSkipped       = "sections_skipped"
)

var OrderedSections = []string{
	SectionRuntime,
	SectionLLM,
	SectionEmbed,
	SectionImage,
	SectionVoice,
	SectionMusic,
}

type RunRequest struct {
	NodeID         string   `json:"node_id,omitempty"`
	ModelID        string   `json:"model_id"`
	Type           string   `json:"type"`
	Sections       []string `json:"sections,omitempty"`
	Iterations     int      `json:"iterations,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type Record struct {
	NodeID   string             `json:"node_id"`
	ModelID  string             `json:"model_id"`
	Latest   *Summary           `json:"latest,omitempty"`
	Sections map[string]Summary `json:"sections,omitempty"`
	History  []Summary          `json:"history,omitempty"`
}

type ModelBenchmark struct {
	Latest   *Summary           `json:"latest,omitempty"`
	Sections map[string]Summary `json:"sections,omitempty"`
}

type Summary struct {
	RunID         string         `json:"run_id"`
	Type          string         `json:"type"`
	Section       string         `json:"section"`
	Status        string         `json:"status"`
	StartedAt     int64          `json:"started_at"`
	FinishedAt    int64          `json:"finished_at"`
	DurationMS    int64          `json:"duration_ms"`
	Metrics       []Metric       `json:"metrics,omitempty"`
	Error         string         `json:"error,omitempty"`
	OptionChanges []OptionChange `json:"option_changes,omitempty"`
}

type Metric struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	Value      float64 `json:"value,omitempty"`
	Unit       string  `json:"unit,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type OptionChange struct {
	Key      string          `json:"key"`
	Kind     string          `json:"kind"`
	Previous json.RawMessage `json:"previous,omitempty"`
	Current  json.RawMessage `json:"current,omitempty"`
}
