import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AnalyticsResponse } from "../types";

const rendered = new Map<string, {innerHTML: string; value: string; checked: boolean; textContent: string}>();

vi.mock("../elements", () => ({
  elements: new Proxy({}, {
    get: (_target, name: string) => {
      if (!rendered.has(name)) {
        rendered.set(name, {innerHTML: "", value: "", checked: false, textContent: ""});
      }
      return rendered.get(name);
    }
  })
}));

const { state } = await import("../state");
const { renderAnalytics } = await import("../analytics");

function embeddingAnalytics(): AnalyticsResponse {
  return {
    enabled: true,
    from: 0,
    to: 1,
    granularity: "hour",
    filters: {node_ids: ["local"], model_ids: ["embed"]},
    summary: {
      request_count: 1,
      success_count: 1,
      failure_count: 0,
      input_tokens: 1276,
      output_tokens: 0,
      total_tokens: 1276,
      image_count: 0,
      embedding_count: 150,
      audio_seconds: 0,
      audio_tokens: 0,
      average_duration_ms: 900,
      average_tokens_per_second: 0,
      load_count: 0,
      average_load_duration_ms: 0,
      vram_peak_mb: 0,
      vram_peak_percent: 0,
      vram_total_mb: 0,
      model_vram_estimate_mb: 0
    },
    timeline: [],
    sections: [],
    models: [{
      node_id: "local",
      model_id: "embed",
      request_count: 1,
      total_tokens: 1276,
      image_count: 0,
      embedding_count: 150,
      audio_seconds: 0,
      load_count: 0,
      average_load_duration_ms: 0,
      vram_peak_mb: 0,
      vram_peak_percent: 0,
      model_vram_estimate_mb: 0
    }],
    nodes: [{
      node_id: "local",
      request_count: 1,
      total_tokens: 1276,
      image_count: 0,
      embedding_count: 150,
      audio_seconds: 0,
      load_count: 0,
      average_load_duration_ms: 0,
      vram_peak_mb: 0,
      vram_peak_percent: 0,
      model_vram_estimate_mb: 0
    }],
    recent: [{
      node_id: "local",
      model_id: "embed",
      section: "embed",
      backend_mode: "llama_sdcpp",
      event_type: "request",
      route: "/v1/embeddings",
      status_code: 200,
      success: true,
      started_at: 0,
      finished_at: 1,
      duration_ms: 900,
      input_tokens: 1276,
      embedding_count: 150
    }]
  };
}

describe("analytics embedding vectors", () => {
  beforeEach(() => {
    rendered.clear();
    state.analytics.query = {period: "24h"};
    state.analytics.showDetails = false;
    state.analytics.data = embeddingAnalytics();
  });

  it("shows returned vectors in the summary, the tables and the recent row", () => {
    renderAnalytics();
    expect(rendered.get("analyticsSummary")?.innerHTML).toContain("Vectors");
    expect(rendered.get("analyticsSummary")?.innerHTML).toContain("150");
    expect(rendered.get("analyticsModelsTable")?.innerHTML).toContain("150");
    expect(rendered.get("analyticsNodesTable")?.innerHTML).toContain("150");
    expect(rendered.get("analyticsRecentTable")?.innerHTML).toContain("1,276 in / 150 vectors");
  });
});
