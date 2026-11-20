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
  gated?: boolean;
  skipped?: {path: string; reason: string}[];
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

export interface UnhashedFile {
  path: string;
  size: number;
}

export interface DownloadLibraryResponse {
  artifacts: ArtifactRecord[];
  unhashed?: UnhashedFile[];
  jobs: DownloadJob[];
}
