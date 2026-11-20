import type { JsonValue } from "../json";

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

export interface LoadErrorRecord {
  id: string;
  fingerprint: string;
  first_seen_at: string;
  last_seen_at: string;
  occurrences: number;
  node_id?: string;
  model_id?: string;
  config_name?: string;
  backend?: string;
  backend_mode?: string;
  phase: string;
  severity: string;
  source?: string;
  message: string;
  exit_error?: string;
  output?: string;
  truncated?: boolean;
}

export interface LoadErrorClearResponse {
  cleared: number;
  cleared_nodes: string[];
  node_errors?: {node_id: string; error: string}[];
}

export interface LoadErrorListResponse {
  enabled: boolean;
  nodes: {node_id: string; enabled: boolean}[];
  records: LoadErrorRecord[];
  node_errors?: {node_id: string; error: string}[];
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
