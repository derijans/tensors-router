import { state } from "./state";
import { jsonRecord } from "./json";
import type {
  ConfigFileRequest,
  ConfigFileResponse,
  BenchmarkRecord,
  BenchmarkRunRequest,
  CookRequest,
  CookResponse,
  ErrorResponse,
  InventoryResponse,
  RouterProcessStatus,
  SessionResponse,
  ValidationIssue
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

export function killRouter(): Promise<RouterProcessStatus> {
  return api<RouterProcessStatus>("/api/router/kill", {method: "POST"});
}

export function getInventory(): Promise<InventoryResponse> {
  return api<InventoryResponse>("/api/inventory");
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
