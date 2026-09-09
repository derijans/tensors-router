import { readFileSync } from "node:fs";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AnalyticsFlushResponse, AnalyticsResponse } from "../types";

const flushAnalytics = vi.fn();
const getAnalytics = vi.fn();

vi.mock("../api", () => ({
  flushAnalytics: (): Promise<AnalyticsFlushResponse> => flushAnalytics() as Promise<AnalyticsFlushResponse>,
  getAnalytics: (): Promise<AnalyticsResponse> => getAnalytics() as Promise<AnalyticsResponse>
}));

vi.mock("../elements", () => ({
  elements: new Proxy({}, {get: () => ({innerHTML: "", value: "", options: [], appendChild: () => undefined})})
}));

const page = readFileSync(new URL("../../index.html", import.meta.url), "utf8");

describe("analytics flush control", () => {
  beforeEach(() => {
    flushAnalytics.mockReset();
    getAnalytics.mockReset();
    getAnalytics.mockResolvedValue({
      enabled: true,
      from: 0,
      to: 0,
      granularity: "hour",
      summary: {},
      timeline: [],
      sections: [],
      models: [],
      nodes: [],
      recent: [],
      filters: {node_ids: [], model_ids: [], sections: []}
    });
  });

  it("is offered as a button in the analytics panel head", () => {
    expect(page).toMatch(/<button id="analyticsFlushButton"[^>]*data-operation-group="analytics"[^>]*>Flush to disk<\/button>/);
  });

  it("reloads analytics after the router persists the buffer", async () => {
    flushAnalytics.mockResolvedValue({flushed_nodes: ["node-a"]});
    const { flushAnalyticsToDisk } = await import("../analytics");

    await flushAnalyticsToDisk();

    expect(flushAnalytics).toHaveBeenCalledTimes(1);
    expect(getAnalytics).toHaveBeenCalledTimes(1);
  });

  it("surfaces a node that could not persist its buffer", async () => {
    flushAnalytics.mockResolvedValue({flushed_nodes: [], node_errors: [{node_id: "node-b", error: "disk full"}]});
    const { flushAnalyticsToDisk } = await import("../analytics");

    await expect(flushAnalyticsToDisk()).rejects.toThrow("node-b: disk full");
    expect(getAnalytics).not.toHaveBeenCalled();
  });
});
