import type { LaneKind } from "./common";

export interface WebUICompatibleModel {
  id: string;
  model_id: string;
  local_id?: string;
  image_id?: string;
  local_image_id?: string;
  filename: string;
  node_id: string;
  node_url?: string;
  backend_mode: string;
  active: boolean;
}

export interface WebUIEntry {
  id: string;
  name: string;
  backend: string;
  backend_mode: string;
  lane: LaneKind;
  url: string;
  node_id: string;
  node_url?: string;
  enabled: boolean;
  active: boolean;
  active_model_id?: string;
  active_image_id?: string;
  requires_loaded_model: boolean;
  can_open_without_model: boolean;
  compatible_models: WebUICompatibleModel[];
}

export interface WebUICatalogResponse {
  object: "list";
  data: WebUIEntry[];
}

export interface WebUISessionRequest {
  id: string;
  enabled: boolean;
}

export interface WebUILoadRequest {
  id: string;
  model_id?: string;
  image_id?: string;
}

export interface WebUILoadResponse {
  ok: boolean;
  id: string;
  url: string;
  model_id?: string;
  image_id?: string;
}
