package analytics

import "time"

const (
	SectionLLM   = "llm"
	SectionEmbed = "embed"
	SectionImage = "image"
	SectionVoice = "voice"
	SectionMusic = "music"
)

const (
	EventTypeRequest   = "request"
	EventTypeModelLoad = "model_load"
)

type EventSink interface {
	Record(Event)
}

type Event struct {
	NodeID          string    `json:"node_id"`
	ModelID         string    `json:"model_id"`
	Section         string    `json:"section"`
	BackendMode     string    `json:"backend_mode"`
	EventType       string    `json:"event_type"`
	Route           string    `json:"route"`
	ConfigFilename  string    `json:"config_filename,omitempty"`
	StatusCode      int       `json:"status_code"`
	Success         bool      `json:"success"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	DurationMS      int64     `json:"duration_ms"`
	InputTokens     int64     `json:"input_tokens,omitempty"`
	OutputTokens    int64     `json:"output_tokens,omitempty"`
	TotalTokens     int64     `json:"total_tokens,omitempty"`
	TokensPerSecond float64   `json:"tokens_per_second,omitempty"`
	ImageCount      int64     `json:"image_count,omitempty"`
	ImageWidth      int64     `json:"image_width,omitempty"`
	ImageHeight     int64     `json:"image_height,omitempty"`
	ImageType       string    `json:"image_type,omitempty"`
	AudioSeconds    float64   `json:"audio_seconds,omitempty"`
	AudioTokens     int64     `json:"audio_tokens,omitempty"`
	LoadVRAMBefore  int64     `json:"load_vram_before_mb,omitempty"`
	LoadVRAMAfter   int64     `json:"load_vram_after_mb,omitempty"`
	LoadVRAMDelta   int64     `json:"load_vram_delta_mb,omitempty"`
	WorkVRAMStart   int64     `json:"work_vram_start_mb,omitempty"`
	WorkVRAMMax     int64     `json:"work_vram_max_mb,omitempty"`
	WorkVRAMEnd     int64     `json:"work_vram_end_mb,omitempty"`
	ModelVRAM       int64     `json:"model_vram_estimate_mb,omitempty"`
	VRAMTotal       int64     `json:"vram_total_mb,omitempty"`
	VRAMPeakPercent float64   `json:"vram_peak_percent,omitempty"`
}

type Query struct {
	Period  string
	StartMS int64
	EndMS   int64
	NodeID  string
	ModelID string
	Section string
}

type Response struct {
	Enabled     bool           `json:"enabled"`
	From        int64          `json:"from"`
	To          int64          `json:"to"`
	Granularity string         `json:"granularity"`
	Summary     Summary        `json:"summary"`
	Timeline    []Timeline     `json:"timeline"`
	Sections    []SectionUsage `json:"sections"`
	Models      []ModelUsage   `json:"models"`
	Nodes       []NodeUsage    `json:"nodes"`
	Recent      []RecentEvent  `json:"recent"`
	NodeErrors  []NodeError    `json:"node_errors,omitempty"`
}

type Summary struct {
	RequestCount    int64   `json:"request_count"`
	SuccessCount    int64   `json:"success_count"`
	FailureCount    int64   `json:"failure_count"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	ImageCount      int64   `json:"image_count"`
	AudioSeconds    float64 `json:"audio_seconds"`
	AudioTokens     int64   `json:"audio_tokens"`
	AverageDuration float64 `json:"average_duration_ms"`
	AverageTokensPS float64 `json:"average_tokens_per_second"`
	LoadCount       int64   `json:"load_count"`
	AverageLoadMS   float64 `json:"average_load_duration_ms"`
	VRAMPeakMB      int64   `json:"vram_peak_mb"`
	VRAMPeakPercent float64 `json:"vram_peak_percent"`
	VRAMTotalMB     int64   `json:"vram_total_mb"`
	ModelVRAMMB     int64   `json:"model_vram_estimate_mb"`
}

type Timeline struct {
	BucketStart  int64   `json:"bucket_start"`
	RequestCount int64   `json:"request_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	ImageCount   int64   `json:"image_count"`
	AudioSeconds float64 `json:"audio_seconds"`
	LoadCount    int64   `json:"load_count"`
	VRAMPeakMB   int64   `json:"vram_peak_mb"`
	VRAMPeakPct  float64 `json:"vram_peak_percent"`
	VRAMTotalMB  int64   `json:"vram_total_mb"`
	ModelVRAMMB  int64   `json:"model_vram_estimate_mb"`
}

type SectionUsage struct {
	Section      string  `json:"section"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	ImageCount   int64   `json:"image_count"`
	AudioSeconds float64 `json:"audio_seconds"`
	LoadCount    int64   `json:"load_count"`
	VRAMPeakMB   int64   `json:"vram_peak_mb"`
	VRAMPeakPct  float64 `json:"vram_peak_percent"`
	ModelVRAMMB  int64   `json:"model_vram_estimate_mb"`
}

type ModelUsage struct {
	NodeID        string  `json:"node_id"`
	ModelID       string  `json:"model_id"`
	RequestCount  int64   `json:"request_count"`
	TotalTokens   int64   `json:"total_tokens"`
	ImageCount    int64   `json:"image_count"`
	AudioSeconds  float64 `json:"audio_seconds"`
	LoadCount     int64   `json:"load_count"`
	AverageLoadMS float64 `json:"average_load_duration_ms"`
	VRAMPeakMB    int64   `json:"vram_peak_mb"`
	VRAMPeakPct   float64 `json:"vram_peak_percent"`
	ModelVRAMMB   int64   `json:"model_vram_estimate_mb"`
}

type NodeUsage struct {
	NodeID        string  `json:"node_id"`
	RequestCount  int64   `json:"request_count"`
	TotalTokens   int64   `json:"total_tokens"`
	ImageCount    int64   `json:"image_count"`
	AudioSeconds  float64 `json:"audio_seconds"`
	LoadCount     int64   `json:"load_count"`
	AverageLoadMS float64 `json:"average_load_duration_ms"`
	VRAMPeakMB    int64   `json:"vram_peak_mb"`
	VRAMPeakPct   float64 `json:"vram_peak_percent"`
	ModelVRAMMB   int64   `json:"model_vram_estimate_mb"`
}

type RecentEvent struct {
	NodeID          string  `json:"node_id"`
	ModelID         string  `json:"model_id"`
	Section         string  `json:"section"`
	BackendMode     string  `json:"backend_mode"`
	EventType       string  `json:"event_type"`
	Route           string  `json:"route"`
	ConfigFilename  string  `json:"config_filename,omitempty"`
	StatusCode      int     `json:"status_code"`
	Success         bool    `json:"success"`
	StartedAt       int64   `json:"started_at"`
	FinishedAt      int64   `json:"finished_at"`
	DurationMS      int64   `json:"duration_ms"`
	InputTokens     int64   `json:"input_tokens,omitempty"`
	OutputTokens    int64   `json:"output_tokens,omitempty"`
	TotalTokens     int64   `json:"total_tokens,omitempty"`
	TokensPerSecond float64 `json:"tokens_per_second,omitempty"`
	ImageCount      int64   `json:"image_count,omitempty"`
	ImageWidth      int64   `json:"image_width,omitempty"`
	ImageHeight     int64   `json:"image_height,omitempty"`
	ImageType       string  `json:"image_type,omitempty"`
	AudioSeconds    float64 `json:"audio_seconds,omitempty"`
	AudioTokens     int64   `json:"audio_tokens,omitempty"`
	LoadVRAMBefore  int64   `json:"load_vram_before_mb,omitempty"`
	LoadVRAMAfter   int64   `json:"load_vram_after_mb,omitempty"`
	LoadVRAMDelta   int64   `json:"load_vram_delta_mb,omitempty"`
	WorkVRAMStart   int64   `json:"work_vram_start_mb,omitempty"`
	WorkVRAMMax     int64   `json:"work_vram_max_mb,omitempty"`
	WorkVRAMEnd     int64   `json:"work_vram_end_mb,omitempty"`
	ModelVRAM       int64   `json:"model_vram_estimate_mb,omitempty"`
	VRAMTotal       int64   `json:"vram_total_mb,omitempty"`
	VRAMPeakPercent float64 `json:"vram_peak_percent,omitempty"`
}

type NodeError struct {
	NodeID  string `json:"node_id,omitempty"`
	NodeURL string `json:"node_url,omitempty"`
	Error   string `json:"error"`
}
