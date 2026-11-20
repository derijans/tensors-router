import { api } from "./client";
import type { RouterProcessStatus, SessionResponse } from "../types";

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
