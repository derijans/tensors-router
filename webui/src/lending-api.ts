import { api } from "./api";
import type { LendingDecisionQuery, LendingDecisionsResponse, LendingSettingsResponse, LendingSummaryResponse } from "./types";

const decisionLimit = 1000;

export function getLendingSettings(): Promise<LendingSettingsResponse> {
  return api<LendingSettingsResponse>("/api/offload/settings");
}

export function saveLendingSettings(values: Record<string, string>): Promise<LendingSettingsResponse> {
  return api<LendingSettingsResponse>("/api/offload/settings", {method: "POST", body: JSON.stringify({values})});
}

export function resetLendingSetting(key: string): Promise<LendingSettingsResponse> {
  return api<LendingSettingsResponse>(`/api/offload/settings?${new URLSearchParams({key}).toString()}`, {method: "DELETE"});
}

export function resetAllLendingSettings(): Promise<LendingSettingsResponse> {
  return api<LendingSettingsResponse>("/api/offload/settings", {method: "DELETE"});
}

export function getLendingDecisions(query: LendingDecisionQuery): Promise<LendingDecisionsResponse> {
  const params = new URLSearchParams({limit: String(decisionLimit)});
  if (query.sinceMilliseconds !== null) params.set("since_ms", String(query.sinceMilliseconds));
  if (query.lane) params.set("lane", query.lane);
  if (query.outcome) params.set("outcome", query.outcome);
  return api<LendingDecisionsResponse>(`/api/offload/decisions?${params.toString()}`);
}

export function getLendingSummary(): Promise<LendingSummaryResponse> {
  return api<LendingSummaryResponse>("/api/offload/summary");
}
