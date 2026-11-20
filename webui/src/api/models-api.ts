import { api } from "./client";
import type { InventoryResponse, ModelStateRequest, LoadConfigRequest, RoutingEndpoint, RoutingLane, RoutingLinksRequest, RoutingLinksResponse, SeparateRuntimeRequest, SeparateRuntimeResponse } from "../types";

export function getInventory(includeFiles = false): Promise<InventoryResponse> {
  const suffix = includeFiles ? "?include_files=true" : "";
  return api<InventoryResponse>(`/api/inventory${suffix}`);
}

export function updateModelState(request: ModelStateRequest): Promise<unknown> {
  return api("/api/models/state", {method: "POST", body: JSON.stringify(request)});
}

const routingLinksPaths: Record<RoutingLane, string> = {
  image: "/api/routing-groups",
  text: "/api/text-routing-groups"
};

function anchorQuery(anchor: RoutingEndpoint): string {
  return `?node_id=${encodeURIComponent(anchor.node_id)}&model_id=${encodeURIComponent(anchor.model_id)}`;
}

export function fetchRoutingLinks(lane: RoutingLane, anchor?: RoutingEndpoint): Promise<RoutingLinksResponse> {
  return api<RoutingLinksResponse>(`${routingLinksPaths[lane]}${anchor ? anchorQuery(anchor) : ""}`);
}

export function saveRoutingLinks(lane: RoutingLane, request: RoutingLinksRequest): Promise<RoutingLinksResponse> {
  return api<RoutingLinksResponse>(routingLinksPaths[lane], {method: "POST", body: JSON.stringify(request)});
}

export function deleteRoutingLinks(lane: RoutingLane, anchor: RoutingEndpoint): Promise<unknown> {
  return api(`${routingLinksPaths[lane]}${anchorQuery(anchor)}`, {method: "DELETE"});
}

export function fetchSeparateRuntime(nodeId: string, localId: string): Promise<SeparateRuntimeResponse> {
  const query = `?node_id=${encodeURIComponent(nodeId)}&local_id=${encodeURIComponent(localId)}`;
  return api<SeparateRuntimeResponse>(`/api/separate-runtimes${query}`);
}

export function saveSeparateRuntime(request: SeparateRuntimeRequest): Promise<SeparateRuntimeResponse> {
  return api<SeparateRuntimeResponse>("/api/separate-runtimes", {method: "POST", body: JSON.stringify(request)});
}

export function loadModelConfig(request: LoadConfigRequest): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>("/api/load", {
    method: "POST",
    body: JSON.stringify(request)
  });
}
