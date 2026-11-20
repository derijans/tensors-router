export type RoutingLane = "image" | "text";

export type RoutingDirection = "lends_to" | "borrows_from";

export interface RoutingEndpoint {
  node_id: string;
  model_id: string;
}

export interface RoutingLink {
  owner: RoutingEndpoint;
  helper: RoutingEndpoint;
  load_if_unloaded: boolean;
  restore_after_borrow: boolean;
}

export interface RoutingLinkState {
  selected: boolean;
  load_if_unloaded: boolean;
  restore_after_borrow: boolean;
}

export interface RoutingCandidate extends RoutingEndpoint {
  filename: string;
  model_hash?: string;
  config_hash?: string;
  context_size?: number;
  multimodal?: boolean;
  weights_match: boolean;
  eligible: boolean;
  ineligible_reason?: string;
  lends_to: RoutingLinkState;
  borrows_from: RoutingLinkState;
}

export interface RoutingLinksResponse {
  links: RoutingLink[];
  anchor?: RoutingEndpoint;
  candidates?: RoutingCandidate[];
}

export interface RoutingLinkChoice extends RoutingEndpoint {
  load_if_unloaded: boolean;
  restore_after_borrow: boolean;
}

export interface RoutingLinksRequest {
  anchor: RoutingEndpoint;
  lends_to: RoutingLinkChoice[];
  borrows_from: RoutingLinkChoice[];
}
