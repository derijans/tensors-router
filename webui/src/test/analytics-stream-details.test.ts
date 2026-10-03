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

function streamedGenerationAnalytics(): AnalyticsResponse {
  return {
    enabled: true,
    from: 0,
    to: 1,
    granularity: "hour",
    filters: {node_ids: ["master"], model_ids: ["think"]},
    summary: {
      request_count: 1,
      success_count: 1,
      failure_count: 0,
      input_tokens: 14925,
      output_tokens: 476,
      total_tokens: 15401,
      image_count: 0,
      embedding_count: 0,
      audio_seconds: 0,
      audio_tokens: 0,
      average_duration_ms: 26376,
      average_tokens_per_second: 28.7,
      average_prompt_tokens_per_second: 0,
      load_count: 0,
      average_load_duration_ms: 0,
      vram_peak_mb: 0,
      vram_peak_percent: 0,
      vram_total_mb: 0,
      model_vram_estimate_mb: 0
    },
    timeline: [],
    sections: [],
    models: [],
    nodes: [],
    recent: [{
      node_id: "master",
      model_id: "think",
      section: "llm",
      backend_mode: "kobold",
      event_type: "request",
      route: "/v1/chat/*",
      status_code: 200,
      success: true,
      started_at: 0,
      finished_at: 1,
      duration_ms: 26376,
      input_tokens: 14925,
      output_tokens: 476,
      total_tokens: 15401,
      ttft_ms: 9743,
      decode_ms: 16610,
      finish_reason: "stop"
    }]
  };
}

describe("analytics stream metrics", () => {
  beforeEach(() => {
    rendered.clear();
    state.analytics.query = {period: "24h"};
    state.analytics.data = streamedGenerationAnalytics();
  });

  it("keeps a streamed generation's token counts beside its stream metrics", () => {
    state.analytics.showDetails = true;
    renderAnalytics();
    const recent = rendered.get("analyticsRecentTable")?.innerHTML ?? "";
    expect(recent).toContain("TTFT 9,743ms");
    expect(recent).toContain("14,925 in / 476 out");
  });

  it("shows the same token counts with stream metrics hidden", () => {
    state.analytics.showDetails = false;
    renderAnalytics();
    expect(rendered.get("analyticsRecentTable")?.innerHTML).toContain("14,925 in / 476 out");
  });

  it("shows how fast the prompt was processed beside the generation speed", () => {
    const data = streamedGenerationAnalytics();
    data.summary.average_prompt_tokens_per_second = 1531.8;
    data.recent = data.recent.map(event => ({...event, tokens_per_second: 28.7, prompt_tokens_per_second: 1531.8}));
    state.analytics.data = data;
    state.analytics.showDetails = false;
    renderAnalytics();
    expect(rendered.get("analyticsRecentTable")?.innerHTML).toContain("14,925 in / 476 out / 28.7 tok/s / 1,532 tok/s prompt");
    expect(rendered.get("analyticsSummary")?.innerHTML).toContain("1,532 tok/s prompt");
  });
});
