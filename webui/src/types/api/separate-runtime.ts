export interface SeparateRuntimeSettings {
  run_separate: boolean;
  triggers: string[];
}

export interface SeparateRuntimeCandidates {
  lanes: string[];
  families: string[];
  configs: string[];
}

export interface SeparateRuntimeResponse {
  node_id: string;
  local_id: string;
  settings: SeparateRuntimeSettings;
  has_override: boolean;
  inherited: SeparateRuntimeSettings;
  candidates: SeparateRuntimeCandidates;
}

export interface SeparateRuntimeRequest {
  node_id: string;
  local_id: string;
  settings: SeparateRuntimeSettings;
}
