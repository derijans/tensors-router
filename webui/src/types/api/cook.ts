import type { Options } from "../json";
import type { LaneKind } from "./common";

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
