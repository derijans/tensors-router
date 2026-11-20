export type MemoryFitLevel = "unknown" | "green" | "yellow" | "light_amber" | "deep_amber" | "red";

export type MemoryFitTier = "unknown" | "measured" | "context_scaled" | "architecture";

export interface MemoryFitBand {
  low_mb: number;
  high_mb: number;
}

export interface ConfigMemoryFit {
  model_id: string;
  config_name: string;
  level: MemoryFitLevel;
  tier: MemoryFitTier;
  vram: MemoryFitBand;
  ram: MemoryFitBand;
  gpu_free_mb: number;
  ram_free_mb: number;
  spill_mb?: number;
  ram_only: boolean;
  multi_gpu?: boolean;
  unavailable?: string;
}

export interface NodeMemoryFit {
  node_id: string;
  configs: ConfigMemoryFit[];
}
