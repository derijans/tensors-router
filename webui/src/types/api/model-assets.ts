export interface ModelAssetResolutionJob {
  id: string;
  config_id: string;
  node_id: string;
  state: "queued" | "resolving" | "completed" | "failed";
  source?: string;
  error?: string;
  results?: {field: string; hash: string; resolved: boolean; failure?: string; source?: string; verification?: string; commit?: string}[];
}

export interface ModelAssetCandidate {
  repository: string;
  repository_path: string;
  commit: string;
  sha256?: string;
  state: "exact" | "mismatched" | "unverifiable";
}
