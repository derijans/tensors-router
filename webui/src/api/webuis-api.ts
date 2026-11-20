import { api } from "./client";
import type { WebUICatalogResponse, WebUILoadRequest, WebUILoadResponse, WebUISessionRequest } from "../types";

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
