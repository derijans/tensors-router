import type { JsonValue, Options } from "./json";

export type LaneKind = "text" | "image" | "embeddings" | "voice" | "music";

export interface SessionResponse {
  authenticated: boolean;
  csrf: string;
}

export interface RouterProcessStatus {
  managed: boolean;
  running: boolean;
  url: string;
  pid?: number;
  can_shutdown: boolean;
  can_force_kill: boolean;
  error?: string;
}

export type BenchmarkType = "general" | "section";

export type BenchmarkSection = "runtime" | "llm" | "embed" | "image" | "voice" | "music";

export interface BenchmarkMetric {
  name: string;
  status: string;
  duration_ms?: number;
  value?: number;
  unit?: string;
  error?: string;
}

export interface BenchmarkOptionChange {
  key: string;
  kind: string;
  previous?: JsonValue;
  current?: JsonValue;
}

export interface BenchmarkSummary {
  run_id: string;
  type: BenchmarkType;
  section: string;
  status: string;
  started_at: number;
  finished_at: number;
  duration_ms: number;
  metrics?: BenchmarkMetric[];
  error?: string;
  option_changes?: BenchmarkOptionChange[];
}

export interface ModelBenchmark {
  latest?: BenchmarkSummary;
  sections?: Record<string, BenchmarkSummary>;
}

export interface BenchmarkRecord extends ModelBenchmark {
  node_id: string;
  model_id: string;
  history?: BenchmarkSummary[];
}

export interface BenchmarkRunRequest {
  node_id?: string;
  model_id: string;
  type: BenchmarkType;
  sections?: string[];
  iterations?: number;
  timeout_seconds?: number;
}

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
  audio_seconds: number;
  audio_tokens: number;
  average_duration_ms: number;
  average_tokens_per_second: number;
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
  audio_seconds: number;
  load_count: number;
  vram_peak_mb: number;
  vram_peak_percent: number;
  vram_total_mb: number;
  model_vram_estimate_mb: number;
}

export interface AnalyticsSectionUsage {
  section: string;
  request_count: number;
  total_tokens: number;
  image_count: number;
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
  image_count?: number;
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
}

export interface AnalyticsNodeError {
  node_id?: string;
  node_url?: string;
  error: string;
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
  node_errors?: AnalyticsNodeError[];
}

export interface LoadConfigRequest {
  model: string;
}

export interface HardwareInfo {
  max_threads: number;
  gpu_backend: string;
  gpu_count: number;
}

export interface ImageCapabilities {
  model?: string;
  upscaler?: string;
  vae?: string;
  vae_auto?: boolean;
  t5xxl?: string;
  clip1?: string;
  clip2?: string;
  clip_l?: string;
  clip_g?: string;
  lora?: string[];
  quant?: number;
  tiled_vae?: number;
  flash_attention?: boolean;
  offload_cpu?: boolean;
  vae_cpu?: boolean;
  clip_gpu?: boolean;
}

export interface EmbeddingCapability {
  model?: string;
  max_ctx?: number;
  gpu?: boolean;
  separate: boolean;
}

export interface MultimodalCapability {
  projector?: string;
  vision_max_res?: number;
  min_tokens?: number;
  max_tokens?: number;
}

export interface VoiceCapability {
  whisper_model?: string;
  tts_model?: string;
  wav_tokenizer?: string;
  directory?: string;
  gpu?: boolean;
}

export interface MusicCapability {
  llm?: string;
  embeddings?: string;
  diffusion?: string;
  vae?: string;
  low_vram?: boolean;
}

export interface Capabilities {
  llm?: boolean;
  image?: ImageCapabilities;
  embeddings?: EmbeddingCapability;
  multimodal?: MultimodalCapability;
  voice?: VoiceCapability;
  music?: MusicCapability;
  context?: number;
}

export interface Model {
  public_id: string;
  local_id: string;
  image_id?: string;
  public_image_id?: string;
  filename: string;
  created: number;
  has_llm: boolean;
  has_image: boolean;
  has_embeddings: boolean;
  has_multimodal: boolean;
  has_voice: boolean;
  has_music: boolean;
  model_hash: string;
  config_hash: string;
  capabilities: Capabilities;
  options?: Options;
  backend_mode: string;
  source: string;
  node_id: string;
  node_url?: string;
  available: boolean;
  asset_state?: "ready" | "unresolved" | "resolving" | "failed";
  unresolved_fields?: number;
  asset_failure?: string;
  benchmark?: ModelBenchmark;
}

export interface FileRecord {
  path: string;
  basename: string;
  extension: string;
  size: number;
  modified: number;
  node_id: string;
  role: string;
  roles?: string[];
  referenced_by?: string[];
  sha256?: string;
}

export interface NodeInventory {
  node_id: string;
  node_url?: string;
  source: string;
  role: string;
  backend_mode: string;
  available: boolean;
  hardware: HardwareInfo;
  models: Model[];
  files: FileRecord[];
  error?: string;
}

export interface RecipeComponent {
  kind: LaneKind;
  node_id: string;
  node_url?: string;
  model_id?: string;
  image_id?: string;
  config_filename: string;
}

export interface Recipe {
  id: string;
  public_id: string;
  public_image_id?: string;
  created: number;
  text?: RecipeComponent;
  image?: RecipeComponent;
  embeddings?: RecipeComponent;
  voice?: RecipeComponent;
  music?: RecipeComponent;
}

export interface OptionDefinition {
  key: string;
  name: string;
  lane: string;
  section?: string;
  value_type: string;
  choices?: string[];
  backends?: string[];
  native_flag?: string;
  cuda_only?: boolean;
  model_role?: string;
  default?: string;
  source?: string;
  known: boolean;
}

export interface InventoryResponse {
  role: string;
  node_id: string;
  node_url?: string;
  nodes: NodeInventory[];
  models: Model[];
  recipes: Recipe[];
  option_catalog: OptionDefinition[];
  observed_options: OptionDefinition[];
}

export interface WebUICompatibleModel {
  id: string;
  model_id: string;
  local_id?: string;
  image_id?: string;
  local_image_id?: string;
  filename: string;
  node_id: string;
  node_url?: string;
  backend_mode: string;
  active: boolean;
}

export interface WebUIEntry {
  id: string;
  name: string;
  backend: string;
  backend_mode: string;
  lane: LaneKind;
  url: string;
  node_id: string;
  node_url?: string;
  enabled: boolean;
  active: boolean;
  active_model_id?: string;
  active_image_id?: string;
  requires_loaded_model: boolean;
  can_open_without_model: boolean;
  compatible_models: WebUICompatibleModel[];
}

export interface WebUICatalogResponse {
  object: "list";
  data: WebUIEntry[];
}

export interface WebUISessionRequest {
  id: string;
  enabled: boolean;
}

export interface WebUILoadRequest {
  id: string;
  model_id?: string;
  image_id?: string;
}

export interface WebUILoadResponse {
  ok: boolean;
  id: string;
  url: string;
  model_id?: string;
  image_id?: string;
}

export interface CookComponent {
  kind: LaneKind;
  node_id: string;
  node_url?: string;
  source: "config" | "file";
  model_id?: string;
  image_id?: string;
  file_path?: string;
  option_key?: string;
}

export interface CookRequest {
  id: string;
  overwrite: boolean;
  components: CookComponent[];
  options?: Options;
}

export interface ConfigResult {
  node_id: string;
  node_url?: string;
  model_id: string;
  image_id?: string;
  filename: string;
  kinds: LaneKind[];
  reused: boolean;
  would_overwrite?: boolean;
}

export interface CookPlan {
  id: string;
  public_id: string;
  public_image_id?: string;
  requires_master_recipe: boolean;
  configs: ConfigResult[];
}

export interface ValidationIssue {
  severity: string;
  code: string;
  message: string;
  node_id?: string;
  field?: string;
}

export interface CookResponse {
  plan: CookPlan;
  recipe?: Recipe;
  validation?: ValidationIssue[];
}

export interface ConfigFileRequest {
  node_id?: string;
  node_url?: string;
  id?: string;
  filename?: string;
  overwrite: boolean;
  options: Options;
}

export interface ConfigFileResponse {
  node_id: string;
  node_url?: string;
  id: string;
  filename: string;
  would_overwrite?: boolean;
  deleted?: boolean;
  options?: Options;
}

export interface ErrorResponse {
  error?: string | { message?: string };
  validation?: ValidationIssue[];
  raw?: string;
}

export interface DownloadCapability {
  enabled: boolean;
  present: boolean;
  working: boolean;
  available: boolean;
  configured: boolean;
  configured_token: boolean;
  storage_root?: string;
  free_bytes?: number;
  free_space_reserve_bytes?: number;
  reason?: string;
  error?: string;
}

export interface DeviceCapability {
  backend: string;
  device_id: string;
  name: string;
  total_vram_bytes: number;
  architecture?: string;
  backend_version?: string;
  split_offload_supported: boolean;
}

export interface DownloadNodeCapability {
  node_id: string;
  node_url?: string;
  available: boolean;
  capability: DownloadCapability;
  devices: DeviceCapability[];
}

export interface DownloadCapabilitiesResponse {
  available?: boolean;
  nodes: DownloadNodeCapability[];
}

export interface PlannedFile {
  path: string;
  size: number;
  required: boolean;
  reason: string;
  lfs_sha256?: string;
}

export interface DownloadPlan {
  repository: string;
  revision: string;
  commit: string;
  files: PlannedFile[];
  total_bytes: number;
  destination: string;
  unsafe_warning: boolean;
}

export interface JobFile {
  path: string;
  reason: string;
  expected_sha256?: string;
  size: number;
  completed_bytes: number;
  state: string;
  error?: string;
}

export interface DownloadJob {
  id: string;
  node_id?: string;
  repository: string;
  revision: string;
  commit: string;
  state: string;
  total_bytes: number;
  completed_bytes: number;
  error?: string;
  created_at: string;
  updated_at: string;
  files: JobFile[];
}

export interface ArtifactRecord {
  path: string;
  sha256: string;
  size: number;
  repository?: string;
  repository_path?: string;
  revision?: string;
  verification_source: string;
}

export interface DownloadLibraryResponse {
  artifacts: ArtifactRecord[];
  jobs: DownloadJob[];
}

export interface LoadCaptureAttempt {
  id: string;
  node_id: string;
  kind: "physical" | "reuse";
  status: "loading" | "succeeded" | "failed" | "interrupted" | "reused";
  backend_mode: string;
  runtime: string;
  lane: string;
  snapshot_sha256: string;
  physical_attempt_id?: string;
  started_at: string;
  finished_at?: string;
  duration_ms: number;
  failure_class?: string;
  failure_message?: string;
  captured_bytes: number;
  truncated: boolean;
  model_hashes?: string[];
}

export interface LoadCaptureQuery {
  node_ids: string[];
  status: string;
  kind: string;
  backend: string;
  from?: number | undefined;
  to?: number | undefined;
  cursor?: string;
}

export interface LoadCaptureListResponse {
  enabled: boolean;
  nodes: {node_id: string; enabled: boolean}[];
  attempts: LoadCaptureAttempt[];
  next_cursor?: string;
  node_errors?: {node_id: string; error: string}[];
}

export interface LoadCaptureDetailResponse {
  attempt: LoadCaptureAttempt;
  snapshot_sha256: string;
  kcpps: JsonValue;
  assets: {role: string; position: number; sha256: string}[];
}

export interface LoadCaptureOutputChunk {
  sequence: number;
  stream: "stdout" | "stderr";
  offset_ns: number;
  payload: string;
}

export interface LoadCaptureOutputResponse {
  chunks: LoadCaptureOutputChunk[];
  next_sequence?: number;
}
