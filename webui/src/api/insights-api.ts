import { api } from "./client";
import type { AnalyticsQuery, AnalyticsFlushResponse, AnalyticsResponse, BenchmarkRecord, BenchmarkRunRequest, LoadCaptureDetailResponse, LoadCaptureListResponse, LoadCaptureOutputResponse, LoadCaptureQuery, LoadErrorClearResponse, LoadErrorListResponse } from "../types";

export function getBenchmarkRecord(nodeID: string, modelID: string): Promise<BenchmarkRecord> {
  const params = new URLSearchParams({model_id: modelID});
  if (nodeID) {
    params.set("node_id", nodeID);
  }
  return api<BenchmarkRecord>(`/api/benchmarks?${params.toString()}`);
}

export function runBenchmark(request: BenchmarkRunRequest): Promise<BenchmarkRecord> {
  return api<BenchmarkRecord>("/api/benchmarks/run", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function getAnalytics(query: AnalyticsQuery): Promise<AnalyticsResponse> {
  const params = new URLSearchParams({period: query.period});
  if (query.node_id) {
    params.set("node_id", query.node_id);
  }
  if (query.model_id) {
    params.set("model_id", query.model_id);
  }
  if (query.section) {
    params.set("section", query.section);
  }
  return api<AnalyticsResponse>(`/api/analytics?${params.toString()}`);
}

export function clearLoadErrors(): Promise<LoadErrorClearResponse> {
  return api<LoadErrorClearResponse>("/api/load-errors", {method: "DELETE"});
}

export function flushAnalytics(): Promise<AnalyticsFlushResponse> {
  return api<AnalyticsFlushResponse>("/api/analytics/flush", {method: "POST"});
}

export function getLoadCaptures(query: LoadCaptureQuery): Promise<LoadCaptureListResponse> {
  const params = new URLSearchParams();
  query.node_ids.forEach(nodeID => params.append("node_id", nodeID));
  if (query.status) params.set("status", query.status);
  if (query.kind) params.set("kind", query.kind);
  if (query.backend) params.set("backend", query.backend);
  if (query.from) params.set("from", String(query.from));
  if (query.to) params.set("to", String(query.to));
  if (query.cursor) params.set("cursor", query.cursor);
  params.set("limit", "100");
  return api<LoadCaptureListResponse>(`/api/load-captures?${params.toString()}`);
}

export function getLoadCaptureDetail(nodeID: string, attemptID: string): Promise<LoadCaptureDetailResponse> {
  const params = new URLSearchParams({node_id: nodeID});
  return api<LoadCaptureDetailResponse>(`/api/load-captures/${encodeURIComponent(attemptID)}?${params.toString()}`);
}

export function getLoadCaptureOutput(nodeID: string, attemptID: string, afterSequence: number): Promise<LoadCaptureOutputResponse> {
  const params = new URLSearchParams({node_id: nodeID, after_sequence: String(afterSequence)});
  return api<LoadCaptureOutputResponse>(`/api/load-captures/${encodeURIComponent(attemptID)}/output?${params.toString()}`);
}

export function getLoadErrors(query: {phase: string; severity: string}): Promise<LoadErrorListResponse> {
  const params = new URLSearchParams();
  if (query.phase) params.set("phase", query.phase);
  if (query.severity) params.set("severity", query.severity);
  params.set("limit", "200");
  return api<LoadErrorListResponse>(`/api/load-errors?${params.toString()}`);
}
