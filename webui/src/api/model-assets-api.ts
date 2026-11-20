import { api, apiBlob } from "./client";
import type { ModelAssetCandidate, ModelAssetResolutionJob } from "../types";

export function resolveModelAssets(request: {id: string}): Promise<{id: string; filename: string; results: {field: string; hash: string; resolved: boolean; failure?: string}[]}> {
  return api("/api/model-assets/resolve", {method: "POST", body: JSON.stringify(request)});
}

export function hashModelFile(request: {node_id: string; path: string}): Promise<{node_id: string; path: string; sha256: string}> {
  return api("/api/model-files/hash", {method: "POST", body: JSON.stringify(request)});
}

export function resolveModelAssetBatch(requests: {node_id?: string; node_url?: string; id: string; filename?: string}[]): Promise<{id: string; filename: string; results: {field: string; hash: string; resolved: boolean; failure?: string}[]}[]> {
  return api("/api/model-assets/resolve-batch", {method: "POST", body: JSON.stringify(requests)});
}

export function createModelAssetResolutionJob(request: {node_id?: string; node_url?: string; id: string; filename?: string}): Promise<ModelAssetResolutionJob> {
  return api("/api/model-assets/jobs", {method: "POST", body: JSON.stringify(request)});
}

export function getModelAssetResolutionJob(nodeID: string, jobID: string): Promise<ModelAssetResolutionJob> {
  return api(`/api/model-assets/jobs/${encodeURIComponent(jobID)}?node_id=${encodeURIComponent(nodeID)}`);
}

export function findModelAssetCandidates(request: {node_id: string; node_url?: string; sha256: string; filename: string; token?: string}, signal?: AbortSignal): Promise<ModelAssetCandidate[]> {
  return api("/api/model-assets/candidates", {method: "POST", body: JSON.stringify(request), ...(signal ? {signal} : {})});
}

export function bindModelAssetCandidate(request: {node_id: string; sha256: string; repository: string; repository_path: string; commit: string; token?: string}): Promise<{sha256: string; hf: string}> {
  return api("/api/model-assets/bind", {method: "POST", body: JSON.stringify(request)});
}

export function substituteModelAsset(request: {node_id: string; id: string; filename: string; field: string; position?: number; expected_sha256: string; sha256: string; repository: string; repository_path: string; commit: string; token?: string; confirm: true}): Promise<{sha256: string; hf: string}> {
  return api("/api/model-assets/substitute", {method: "POST", body: JSON.stringify(request)});
}

export function lookupModelAsset(hash: string, signal?: AbortSignal): Promise<{sha256: string; available: boolean; filename?: string; size?: number; origin?: string; nodes?: string[]}> {
  return api(`/api/model-assets/${encodeURIComponent(hash)}`, signal ? {signal} : undefined);
}

export function exportPortableConfig(request: {node_id: string; node_url?: string; id: string; filename: string}): Promise<Blob> {
  return apiBlob("/api/model-assets/export", {method: "POST", body: JSON.stringify(request)});
}
