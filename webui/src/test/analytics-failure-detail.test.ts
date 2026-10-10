import { describe, expect, it } from "vitest";
import { failureDetail, queueWaitDetail } from "../analytics-failure-detail";
import type { AnalyticsRecentEvent } from "../types";

function event(overrides: Partial<AnalyticsRecentEvent>): AnalyticsRecentEvent {
  return {node_id: "n", model_id: "m", section: "llm", backend_mode: "kobold", event_type: "request", route: "/v1/chat/*", status_code: 502, success: false, started_at: 0, finished_at: 1, duration_ms: 1, ...overrides};
}

describe("failure detail of a recent request", () => {
  it("names the backend status and reason of a failed request", () => {
    expect(failureDetail(event({upstream_status: 500, error_message: "exceeds the available context size"}))).toBe("backend 500: exceeds the available context size");
  });

  it("shows the router's own reason when the backend never answered", () => {
    expect(failureDetail(event({error_message: "backend unavailable"}))).toBe("backend unavailable");
  });

  it("stays empty for a successful request and for a failure without a recorded reason", () => {
    expect(failureDetail(event({success: true, status_code: 200, error_message: "stale"}))).toBe("");
    expect(failureDetail(event({}))).toBe("");
  });

  it("bounds a long reason", () => {
    expect(failureDetail(event({error_message: "x".repeat(400)})).length).toBe(160);
  });

  it("reports queue wait only when the request waited", () => {
    expect(queueWaitDetail(event({queue_wait_ms: 4200}))).toBe("queued 4200ms");
    expect(queueWaitDetail(event({}))).toBe("");
  });
});
