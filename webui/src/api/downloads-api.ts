import { api } from "./client";
import type { DownloadCapabilitiesResponse, DownloadLibraryResponse, DownloadPlan, DownloadJob } from "../types";

export function getDownloadCapabilities(): Promise<DownloadCapabilitiesResponse> {
  return api<DownloadCapabilitiesResponse>("/api/download/capabilities");
}

export function searchDownloads(request: {node_id: string; query: string; author?: string; pipeline_tag?: string; filters?: string[]; num_parameters?: string; gated?: string; inference?: string; apps?: string[]; inference_providers?: string[]; trained_datasets?: string[]; sort?: string; direction?: string; token?: string}): Promise<{id: string; downloads: number; likes: number; gated?: string}[]> {
  return api<{id: string; downloads: number; likes: number; gated?: string}[]>("/api/download/search", {method: "POST", body: JSON.stringify(request)});
}

export function searchDownloadPage(request: {node_id: string; query: string; author?: string; pipeline_tag?: string; filters?: string[]; num_parameters?: string; gated?: string; inference?: string; apps?: string[]; inference_providers?: string[]; trained_datasets?: string[]; sort?: string; direction?: string; cursor?: string; limit?: number; token?: string}, signal?: AbortSignal): Promise<{results: {id: string; downloads: number; likes: number; gated?: string; tags?: string[]; author?: string; updated_at?: string}[]; next_cursor?: string}> {
  return api("/api/download/search-page", {method: "POST", body: JSON.stringify(request), ...(signal ? {signal} : {})});
}

export function planDownload(request: {node_id: string; repository: string; revision?: string; files: string[]; mode: string; token?: string}): Promise<DownloadPlan> {
  return api<DownloadPlan>("/api/download/plan", {method: "POST", body: JSON.stringify(request)});
}

export function createDownloadJob(request: {node_id: string; repository: string; revision?: string; files: string[]; mode?: "smart" | "explicit"; token?: string; confirm_unsafe: boolean; confirm_replace: boolean}): Promise<DownloadJob> {
  return api<DownloadJob>("/api/download/jobs", {method: "POST", body: JSON.stringify(request)});
}

export function getDownloadLibrary(nodeID: string): Promise<DownloadLibraryResponse> {
  return api<DownloadLibraryResponse>(`/api/download/library?${new URLSearchParams({node_id: nodeID})}`);
}

export function rescanDownloads(nodeID: string): Promise<{artifacts: unknown[]}> {
  return api<{artifacts: unknown[]}>(`/api/download/rescan?${new URLSearchParams({node_id: nodeID})}`, {method: "POST"});
}

export function downloadJobAction(nodeID: string, jobID: string, action: "pause" | "resume" | "cancel"): Promise<DownloadJob> {
  return api<DownloadJob>(`/api/download/jobs/${encodeURIComponent(jobID)}/${action}?${new URLSearchParams({node_id: nodeID})}`, {method: "POST"});
}
