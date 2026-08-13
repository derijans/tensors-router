import { state } from "./state";
import type { LoadCaptureDetailResponse, LoadCaptureListResponse, LoadCaptureOutputResponse, LoadCaptureQuery } from "./types";
import { jsonRecord } from "./json";
import type {
  ConfigFileRequest,
  ConfigFileResponse,
  AnalyticsQuery,
  AnalyticsResponse,
  BenchmarkRecord,
  BenchmarkRunRequest,
  CookRequest,
  CookResponse,
  ErrorResponse,
  InventoryResponse,
  ModelStateRequest,
  BackendInitializationJob,
  BackendInitializationRequest,
  NodeState,
  NodeUnloadRequest,
  LoadConfigRequest,
  RouterProcessStatus,
  SessionResponse,
  ValidationIssue,
  WebUICatalogResponse,
  WebUILoadRequest,
  WebUILoadResponse,
  WebUISessionRequest,
  DownloadCapabilitiesResponse,
  DownloadLibraryResponse,
  DownloadPlan,
  DownloadJob
} from "./types";

export type WebError = Error & { data: unknown };

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (state.csrf && options.method && options.method !== "GET") {
    headers.set("X-CSRF-Token", state.csrf);
  }
  const response = await fetch(path, {...options, headers});
  const text = await response.text();
  const data = parseResponse(text);
  if (!response.ok) {
    throw webError(errorMessage(data, text, response.statusText), data);
  }
  return data as T;
}

export function getSession(): Promise<SessionResponse> {
  return api<SessionResponse>("/api/session");
}

export function login(token: string): Promise<SessionResponse> {
  return api<SessionResponse>("/api/login", {
    method: "POST",
    body: JSON.stringify({token})
  });
}

export function logout(): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>("/api/logout", {method: "POST"});
}

export function getRouterStatus(): Promise<RouterProcessStatus> {
  return api<RouterProcessStatus>("/api/router/status");
}

export function launchRouter(): Promise<RouterProcessStatus> {
  return api<RouterProcessStatus>("/api/router/launch", {method: "POST"});
}

export function restartRouter(): Promise<RouterProcessStatus> {
  return api<RouterProcessStatus>("/api/router/restart", {method: "POST"});
}

export function shutdownRouter(): Promise<RouterProcessStatus> {
  return api<RouterProcessStatus>("/api/router/shutdown", {method: "POST"});
}

export function forceKillRouter(): Promise<RouterProcessStatus> {
  return api<RouterProcessStatus>("/api/router/force-kill", {method: "POST"});
}

export function getInventory(includeFiles = false): Promise<InventoryResponse> {
  const suffix = includeFiles ? "?include_files=true" : "";
  return api<InventoryResponse>(`/api/inventory${suffix}`);
}

export function getNodeState(nodeID: string, signal?: AbortSignal): Promise<NodeState> {
  const path = `/api/nodes/state?${new URLSearchParams({node_id: nodeID})}`;
  return api<NodeState>(path, signal ? {signal} : undefined);
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

export function updateModelState(request: ModelStateRequest): Promise<unknown> {
  return api("/api/models/state", {method: "POST", body: JSON.stringify(request)});
}

export function resolveModelAssets(request: {id: string}): Promise<{id: string; filename: string; results: {field: string; hash: string; resolved: boolean; failure?: string}[]}> {
  return api("/api/model-assets/resolve", {method: "POST", body: JSON.stringify(request)});
}

export function hashModelFile(request: {node_id: string; path: string}): Promise<{node_id: string; path: string; sha256: string}> {
  return api("/api/model-files/hash", {method: "POST", body: JSON.stringify(request)});
}

export function resolveModelAssetBatch(requests: {node_id?: string; node_url?: string; id: string; filename?: string}[]): Promise<{id: string; filename: string; results: {field: string; hash: string; resolved: boolean; failure?: string}[]}[]> {
  return api("/api/model-assets/resolve-batch", {method: "POST", body: JSON.stringify(requests)});
}

export interface ModelAssetResolutionJob {
  id: string;
  config_id: string;
  node_id: string;
  state: "queued" | "resolving" | "completed" | "failed";
  source?: string;
  error?: string;
  results?: {field: string; hash: string; resolved: boolean; failure?: string; source?: string; verification?: string; commit?: string}[];
}

export function createModelAssetResolutionJob(request: {node_id?: string; node_url?: string; id: string; filename?: string}): Promise<ModelAssetResolutionJob> {
  return api("/api/model-assets/jobs", {method: "POST", body: JSON.stringify(request)});
}

export function getModelAssetResolutionJob(nodeID: string, jobID: string): Promise<ModelAssetResolutionJob> {
  return api(`/api/model-assets/jobs/${encodeURIComponent(jobID)}?node_id=${encodeURIComponent(nodeID)}`);
}

export interface ModelAssetCandidate {
  repository: string;
  repository_path: string;
  commit: string;
  sha256?: string;
  state: "exact" | "mismatched" | "unverifiable";
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

export async function exportPortableConfig(request: {node_id: string; node_url?: string; id: string; filename: string}): Promise<Blob> {
  const headers = new Headers({"Content-Type": "application/json"});
  if (state.csrf) {
    headers.set("X-CSRF-Token", state.csrf);
  }
  const response = await fetch("/api/model-assets/export", {method: "POST", headers, body: JSON.stringify(request)});
  if (!response.ok) {
    const content = await response.text();
    throw webError(errorMessage(parseResponse(content), content, response.statusText), parseResponse(content));
  }
  return response.blob();
}

export function getDownloadCapabilities(): Promise<DownloadCapabilitiesResponse> {
  return api<DownloadCapabilitiesResponse>("/api/download/capabilities");
}

export function searchDownloads(request: {node_id: string; query: string; author?: string; pipeline_tag?: string; filters?: string[]; num_parameters?: string; gated?: string; inference?: string; apps?: string[]; inference_providers?: string[]; trained_datasets?: string[]; sort?: string; direction?: string; token?: string}): Promise<{id: string; downloads: number; likes: number; gated?: string}[]> {
  return api<{id: string; downloads: number; likes: number; gated?: string}[]>("/api/download/search", {method: "POST", body: JSON.stringify(request)});
}

export function searchDownloadPage(request: {node_id: string; query: string; author?: string; pipeline_tag?: string; filters?: string[]; num_parameters?: string; gated?: string; inference?: string; apps?: string[]; inference_providers?: string[]; trained_datasets?: string[]; sort?: string; direction?: string; cursor?: string; limit?: number; token?: string}, signal?: AbortSignal): Promise<{results: {id: string; downloads: number; likes: number; gated?: string; tags?: string[]}[]; next_cursor?: string}> {
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

export function getWebUIs(): Promise<WebUICatalogResponse> {
  return api<WebUICatalogResponse>("/api/webuis");
}

export function setWebUISession(request: WebUISessionRequest): Promise<WebUICatalogResponse> {
  return api<WebUICatalogResponse>("/api/webuis/session", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function loadWebUI(request: WebUILoadRequest): Promise<WebUILoadResponse> {
  return api<WebUILoadResponse>("/api/webuis/load", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

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

export function loadModelConfig(request: LoadConfigRequest): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>("/api/load", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function previewCook(request: CookRequest): Promise<CookResponse> {
  return api<CookResponse>("/api/cook/preview", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function applyCook(request: CookRequest): Promise<CookResponse> {
  return api<CookResponse>("/api/cook/apply", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function deleteRecipe(id: string): Promise<{ ok: boolean }> {
  return api<{ ok: boolean }>(`/api/cook/${encodeURIComponent(id)}`, {method: "DELETE"});
}

export function previewConfigFile(request: ConfigFileRequest): Promise<ConfigFileResponse> {
  return api<ConfigFileResponse>("/api/config-file/preview", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function applyConfigFile(request: ConfigFileRequest): Promise<ConfigFileResponse> {
  return api<ConfigFileResponse>("/api/config-file/apply", {
    method: "POST",
    body: JSON.stringify(request)
  });
}

export function deleteConfigFile(request: ConfigFileRequest): Promise<ConfigFileResponse> {
  return api<ConfigFileResponse>("/api/config-file", {
    method: "DELETE",
    body: JSON.stringify(request)
  });
}

export function errorBody(error: unknown): ErrorResponse {
  if (isWebError(error)) {
    const record = jsonRecord(error.data);
    const validation = validationIssues(record?.validation);
    return validation ? {error: error.message, validation} : {error: error.message};
  }
  return {error: error instanceof Error ? error.message : String(error)};
}

export function isWebError(error: unknown): error is WebError {
  return error instanceof Error && "data" in error;
}

function parseResponse(text: string): unknown {
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return {raw: text};
  }
}

function errorMessage(data: unknown, text: string, fallback: string): string {
  const body = jsonRecord(data);
  if (typeof body?.error === "string") {
    return body.error;
  }
  const nested = jsonRecord(body?.error);
  if (typeof nested?.message === "string") {
    return nested.message;
  }
  return text || fallback;
}

function webError(message: string, data: unknown): WebError {
  const error = new Error(message) as WebError;
  error.data = data;
  return error;
}

function validationIssues(value: unknown): ValidationIssue[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const issues = value.filter(isValidationIssue);
  return issues.length > 0 ? issues : null;
}

function isValidationIssue(value: unknown): value is ValidationIssue {
  const record = jsonRecord(value);
  return typeof record?.severity === "string" &&
    typeof record.code === "string" &&
    typeof record.message === "string";
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
