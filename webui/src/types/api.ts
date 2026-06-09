import type { Options } from "./json";

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
  error?: string;
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
