import { api } from "./client";
import type { ConfigFileRequest, ConfigFileResponse, CookRequest, CookResponse } from "../types";

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
