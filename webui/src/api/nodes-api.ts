import { api } from "./client";
import type { BackendInitializationJob, BackendInitializationRequest, BackendLaunchOptions, BackendLaunchOptionsRequest, NodeMemoryFit, NodeState, NodeUnloadRequest } from "../types";

export function getNodeState(nodeID: string, signal?: AbortSignal): Promise<NodeState> {
  const path = `/api/nodes/state?${new URLSearchParams({node_id: nodeID})}`;
  return api<NodeState>(path, signal ? {signal} : undefined);
}

export function getNodeMemoryFit(nodeID: string): Promise<NodeMemoryFit> {
  return api<NodeMemoryFit>(`/api/nodes/memory-fit?${new URLSearchParams({node_id: nodeID})}`);
}

export function unloadNodeRuntime(request: NodeUnloadRequest): Promise<{ok: boolean}> {
  return api<{ok: boolean}>("/api/nodes/unload", {method: "POST", body: JSON.stringify(request)});
}

export function initializeNodeBackend(request: BackendInitializationRequest): Promise<BackendInitializationJob> {
  return api<BackendInitializationJob>("/api/nodes/backends/init", {method: "POST", body: JSON.stringify(request)});
}

export function cancelNodeBackendInitialization(request: BackendInitializationRequest): Promise<BackendInitializationJob> {
  return api<BackendInitializationJob>("/api/nodes/backends/init/cancel", {method: "POST", body: JSON.stringify(request)});
}

export function nodeBackendLaunchOptions(nodeID: string, backendID: string): Promise<BackendLaunchOptions> {
  const query = new URLSearchParams({node_id: nodeID, backend_id: backendID});
  return api<BackendLaunchOptions>(`/api/nodes/backends/launch-options?${query.toString()}`);
}

export function applyNodeBackendLaunchOptions(request: BackendLaunchOptionsRequest): Promise<BackendLaunchOptions> {
  return api<BackendLaunchOptions>("/api/nodes/backends/launch-options", {method: "POST", body: JSON.stringify(request)});
}
