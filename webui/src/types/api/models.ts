import type { Options } from "../json";
import type { ModelBenchmark } from "./benchmarks";
import type { Recipe } from "./cook";
import type { NodeInventory } from "./nodes";

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
  audio_vae?: string;
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
  disabled?: boolean;
}

export interface ModelStateRequest {
  node_id: string;
  local_id: string;
  enabled: boolean;
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
