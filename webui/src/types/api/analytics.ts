export interface AnalyticsQuery {
  period: "24h" | "7d" | "30d" | "90d" | "all";
  node_id?: string;
  model_id?: string;
  section?: string;
}

export interface AnalyticsFilters {
  node_ids: string[];
  model_ids: string[];
}

export interface AnalyticsSummary {
  request_count: number;
  success_count: number;
  failure_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  image_count: number;
  embedding_count: number;
  audio_seconds: number;
  audio_tokens: number;
  average_duration_ms: number;
  average_tokens_per_second: number;
  average_prompt_tokens_per_second: number;
  load_count: number;
  average_load_duration_ms: number;
  vram_peak_mb: number;
  vram_peak_percent: number;
  vram_total_mb: number;
  model_vram_estimate_mb: number;
}

export interface AnalyticsTimeline {
  bucket_start: number;
  request_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  image_count: number;
  embedding_count: number;
  audio_seconds: number;
  load_count: number;
  vram_peak_mb: number;
  vram_peak_percent: number;
  vram_total_mb: number;
  model_vram_estimate_mb: number;
  failure_count?: number;
  average_tokens_per_second?: number;
  tokens_per_second_samples?: number;
  average_prompt_tokens_per_second?: number;
  prompt_tokens_per_second_samples?: number;
  sections?: AnalyticsTimelineSection[];
}

export interface AnalyticsTimelineSection {
  section: string;
  request_count: number;
}

export interface AnalyticsSectionUsage {
  section: string;
  request_count: number;
  total_tokens: number;
  image_count: number;
  embedding_count: number;
  audio_seconds: number;
  load_count: number;
  vram_peak_mb: number;
  vram_peak_percent: number;
  model_vram_estimate_mb: number;
}

export interface AnalyticsModelUsage {
  node_id: string;
  model_id: string;
  request_count: number;
  total_tokens: number;
  image_count: number;
  embedding_count: number;
  audio_seconds: number;
  load_count: number;
  average_load_duration_ms: number;
  vram_peak_mb: number;
  vram_peak_percent: number;
  model_vram_estimate_mb: number;
}

export interface AnalyticsNodeUsage {
  node_id: string;
  request_count: number;
  total_tokens: number;
  image_count: number;
  embedding_count: number;
  audio_seconds: number;
  load_count: number;
  average_load_duration_ms: number;
  vram_peak_mb: number;
  vram_peak_percent: number;
  model_vram_estimate_mb: number;
}

export interface AnalyticsRecentEvent {
  node_id: string;
  model_id: string;
  section: string;
  backend_mode: string;
  event_type: string;
  route: string;
  config_filename?: string;
  status_code: number;
  success: boolean;
  started_at: number;
  finished_at: number;
  duration_ms: number;
  input_tokens?: number;
  output_tokens?: number;
  total_tokens?: number;
  tokens_per_second?: number;
  prompt_tokens_per_second?: number;
  image_count?: number;
  embedding_count?: number;
  image_width?: number;
  image_height?: number;
  image_type?: string;
  audio_seconds?: number;
  audio_tokens?: number;
  audio_language?: string;
  audio_task?: string;
  load_vram_before_mb?: number;
  load_vram_after_mb?: number;
  load_vram_delta_mb?: number;
  work_vram_start_mb?: number;
  work_vram_max_mb?: number;
  work_vram_end_mb?: number;
  model_vram_estimate_mb?: number;
  vram_total_mb?: number;
  vram_peak_percent?: number;
  ttft_ms?: number;
  decode_ms?: number;
  max_gap_ms?: number;
  finish_reason?: string;
  aborted?: boolean;
  error_message?: string;
  upstream_status?: number;
  queue_wait_ms?: number;
}

export interface AnalyticsNodeError {
  node_id?: string;
  node_url?: string;
  error: string;
}

export interface AnalyticsFlushResponse {
  flushed_nodes: string[];
  node_errors?: AnalyticsNodeError[];
}

export interface AnalyticsResponse {
  enabled: boolean;
  from: number;
  to: number;
  granularity: string;
  filters?: AnalyticsFilters;
  summary: AnalyticsSummary;
  timeline: AnalyticsTimeline[];
  sections: AnalyticsSectionUsage[];
  models: AnalyticsModelUsage[];
  nodes: AnalyticsNodeUsage[];
  recent: AnalyticsRecentEvent[];
  router_versions?: AnalyticsVersionUsage[] | null;
  node_errors?: AnalyticsNodeError[];
}

export interface AnalyticsVersionUsage {
  node_id: string;
  router_version: string;
  first_seen: number;
  last_seen: number;
  event_count: number;
}
