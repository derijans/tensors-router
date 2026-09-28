import type { NodeHeldRequest } from "./api";

export type LendingSettingSource = "default" | "config" | "db";

export interface LendingSettingEntry {
  key: string;
  kind: "duration" | "integer" | "boolean";
  description: string;
  default: string;
  config?: string;
  override?: string;
  effective: string;
  source: LendingSettingSource;
}

export interface LendingSettingsResponse {
  entries: LendingSettingEntry[];
  fingerprint: string;
  editable: boolean;
}

export interface LendingDecision {
  id?: number;
  recorded_at: string;
  node_id: string;
  kind: string;
  trigger?: string;
  lane: string;
  owner_node_id?: string;
  owner_model_id?: string;
  helper_node_id?: string;
  helper_model_id?: string;
  outcome: string;
  reason?: string;
  pending_count?: number;
  backlog_count?: number;
  keep_ms?: number;
  switch_ms?: number;
  service_ms?: number;
  owner_job_ms?: number;
  service_source?: string;
  helper_idle_ms?: number;
  slots?: number;
  lent_out?: number;
  borrowed_ahead?: number;
  wait_ms?: number;
  router_version?: string;
}

export interface LendingNodeError {
  node_id: string;
  error: string;
}

export interface LendingDecisionsResponse {
  records: LendingDecision[];
  node_errors?: LendingNodeError[];
}

export interface LendingLease {
  lane: string;
  owner_node_id: string;
  owner_model_id: string;
  helper_node_id: string;
  helper_model_id: string;
  load_helper_model: boolean;
  restore_helper_model: boolean;
  helper_slots: number;
  probe?: boolean;
  expires_at: string;
}

export interface LendingNodeSummary {
  node_id: string;
  build_version: string;
  settings_fingerprint: string;
  held_requests: NodeHeldRequest[] | null;
}

export interface LendingSummaryResponse {
  nodes: LendingNodeSummary[];
  leases: LendingLease[] | null;
  node_errors?: LendingNodeError[];
}

export interface LendingDecisionQuery {
  sinceMilliseconds: number | null;
  lane: string;
  outcome: string;
}
