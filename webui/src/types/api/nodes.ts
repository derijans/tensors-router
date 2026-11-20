import type { FileRecord, HardwareInfo, Model } from "./models";

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
  build_version?: string;
  error?: string;
}

export interface NodeStateModelRow {
  model_id: string;
  lane: string;
  runtime_id: string;
  generation: number;
  memory_estimate_mb?: number;
  borrowed?: boolean;
}

export type NodeMemoryKind = "vram" | "ram";

export interface NodeMemory {
  kind: NodeMemoryKind;
  total_mb: number;
  used_mb: number;
  sampled_at_ms: number;
  devices?: NodeMemoryDevice[];
  system_ram?: NodeSystemRAM;
}

export interface NodeMemoryDevice {
  vendor: string;
  name: string;
  source: string;
  dedicated_used_mb: number;
  dedicated_total_mb: number;
  shared_used_mb?: number;
  usage_known: boolean;
  unified?: boolean;
}

export interface NodeSystemRAM {
  used_mb: number;
  total_mb: number;
}

export type BackendLifecycleState = "companion_missing" | "unsupported" | "needs_init" | "initializing" | "ready" | "failed";

export interface NodeStateBackend {
  id: string;
  display_name: string;
  mode: string;
  lifecycle_state: BackendLifecycleState;
  selected_profile?: string;
  detected_profile?: string;
  runtime_version?: string;
  initialization_job_id?: string;
  initialization_phase?: string;
  initialization_bytes?: number;
  initialization_total_bytes?: number;
  manifest_trust?: string;
  launch_options?: BackendLaunchOptions;
  error?: string;
  retryable?: boolean;
  loaded_models: NodeStateModelRow[];
}

export interface BackendLaunchOptions {
  hf_hub_offline: boolean;
  transformers_offline: boolean;
  hf_datasets_offline: boolean;
}

export interface BackendLaunchOptionsRequest {
  node_id: string;
  backend_id: string;
  options: BackendLaunchOptions;
}

export type NodeHeldRequestState = "queued" | "held" | "lent";

export interface NodeHeldRequest {
  lane: string;
  model_id: string;
  state: NodeHeldRequestState;
  waiting_ms: number;
  helper_node_id?: string;
  helper_model_id?: string;
}

export type NodeRequestState = "running" | "borrowed" | "waiting_for_runtime" | NodeHeldRequestState;

export interface NodeRequest {
  lane: string;
  model_id: string;
  state: NodeRequestState;
  age_ms: number;
  helper_node_id?: string;
  helper_model_id?: string;
}

export interface NodeState {
  node_id: string;
  backends: NodeStateBackend[];
  active_requests: string[] | null;
  held_requests?: NodeHeldRequest[] | null;
  requests?: NodeRequest[] | null;
  // Absent when the node runs a build from before ffmpeg was reported, which
  // a cluster can contain part-way through a rolling upgrade.
  ffmpeg_available?: boolean;
  ffmpeg_path?: string;
  memory?: NodeMemory;
}

export interface NodeUnloadRequest {
  node_id: string;
  backend_id: string;
  runtime_id: string;
  expected_generation: number;
}

export interface BackendInitializationRequest {
  node_id: string;
  backend_id: string;
  profile?: string;
}

export type BackendInitializationJobState = "queued" | "running" | "completed" | "failed" | "cancelled";

export interface BackendInitializationJob {
  job_id: string;
  backend_id: string;
  state: BackendInitializationJobState;
  selected_profile?: string;
  detected_profile?: string;
  phase?: string;
  completed_bytes: number;
  total_bytes: number;
  error?: string;
  retryable?: boolean;
  created_at?: string;
  updated_at?: string;
}
