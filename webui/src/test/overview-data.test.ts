import { describe, expect, it } from "vitest";
import { attentionItems, overviewKpis, recentRequests, type AttentionSources } from "../overview/overview-data";
import type { AnalyticsRecentEvent, AnalyticsResponse, NodeInventory, NodeState } from "../types";

function analytics(overrides: Partial<AnalyticsResponse> = {}): AnalyticsResponse {
  return {
    enabled: true,
    from: 0,
    to: 0,
    granularity: "hour",
    summary: {
      request_count: 200,
      success_count: 190,
      failure_count: 10,
      input_tokens: 0,
      output_tokens: 0,
      total_tokens: 0,
      image_count: 0,
      embedding_count: 0,
      audio_seconds: 0,
      audio_tokens: 0,
      average_duration_ms: 2500,
      average_tokens_per_second: 38.4,
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
    recent: [],
    filters: {node_ids: [], model_ids: []},
    ...overrides
  };
}

function nodeState(kind: "vram" | "ram", usedMB: number, totalMB: number, lentMB = 0): NodeState {
  return {
    node_id: `node-${kind}-${usedMB}`,
    active_requests: [],
    memory: {kind, used_mb: usedMB, total_mb: totalMB, sampled_at_ms: 1},
    backends: [{
      id: "koboldcpp",
      display_name: "KoboldCpp",
      mode: "kobold",
      lifecycle_state: "ready",
      loaded_models: lentMB > 0 ? [{model_id: "flux", lane: "image", runtime_id: "image-0", generation: 1, memory_estimate_mb: lentMB, borrowed: true}] : []
    }]
  };
}

function inventoryNode(nodeID: string, overrides: Partial<NodeInventory> = {}): NodeInventory {
  return {
    node_id: nodeID,
    source: "local",
    role: "slave",
    backend_mode: "kobold",
    available: true,
    hardware: {max_threads: 8, gpu_backend: "cuda", gpu_count: 1},
    models: [],
    files: [],
    build_version: "0.6.14",
    ...overrides
  };
}

function sources(overrides: Partial<AttentionSources> = {}): AttentionSources {
  return {nodes: [], snapshots: {}, loadErrors: [], loadErrorsEnabled: true, failedCaptures: [], analyticsNodeErrors: [], ...overrides};
}

describe("overview figures", () => {
  it("summarises requests, throughput and failures from analytics", () => {
    const [requests, throughput, , failures] = overviewKpis(analytics(), [], 2);

    expect(requests?.value).toBe("200");
    expect(requests?.detail).toBe("95% succeeded");
    expect(throughput?.value).toBe("38.4");
    expect(throughput?.unit).toBe("tok/s");
    expect(throughput?.detail).toBe("2.5 s per request");
    expect(failures?.value).toBe("10");
    expect(failures?.detail).toBe("2 load errors");
    expect(failures?.detailTone).toBe("danger");
  });

  it("leads the throughput detail with prompt processing speed when nodes measure it", () => {
    const measured = analytics();
    measured.summary.average_prompt_tokens_per_second = 1531.8;
    const [, throughput] = overviewKpis(measured, [], 0);

    expect(throughput?.detail).toBe("1,532 tok/s prompt / 2.5 s per request");
  });

  it("names memory by what the nodes report and counts lent work", () => {
    expect(overviewKpis(null, [nodeState("vram", 8192, 24576)], 0)[2]?.label).toBe("VRAM in use");
    expect(overviewKpis(null, [nodeState("ram", 8192, 65536)], 0)[2]?.label).toBe("RAM in use");
    const mixed = overviewKpis(null, [nodeState("vram", 8192, 24576, 4096), nodeState("ram", 8192, 65536)], 0)[2];
    expect(mixed?.label).toBe("Memory in use");
    expect(mixed?.value).toBe("16.0");
    expect(mixed?.unit).toBe("/ 88.0 GB");
    expect(mixed?.detail).toBe("4.0 GB running lent work");
  });

  it("shows dashes instead of zeros when analytics is off", () => {
    const [requests, , memory] = overviewKpis(analytics({enabled: false}), [], 0);

    expect(requests?.value).toBe("—");
    expect(requests?.detail).toBe("Analytics is off");
    expect(memory?.detail).toBe("No node reports memory");
  });
});

describe("needs attention", () => {
  it("lists load errors before warnings and links each to its error", () => {
    const items = attentionItems(sources({loadErrors: [
      {id: "w", fingerprint: "", first_seen_at: "", last_seen_at: "2026-10-01T10:00:00Z", occurrences: 1, phase: "spawn", severity: "warning", message: "slow start"},
      {id: "e", fingerprint: "", first_seen_at: "", last_seen_at: "2026-10-01T09:00:00Z", occurrences: 3, node_id: "forge", model_id: "sdxl", phase: "load", severity: "error", message: "CUDA out of memory"}
    ]}));

    expect(items.map(item => item.key)).toEqual(["load-error-e", "load-error-w"]);
    expect(items[0]).toMatchObject({tone: "danger", title: "sdxl failed on forge", detail: "CUDA out of memory · 3×", target: {kind: "load-error", id: "e"}});
  });

  it("ignores load errors when the router does not collect them", () => {
    const items = attentionItems(sources({loadErrorsEnabled: false, loadErrors: [
      {id: "e", fingerprint: "", first_seen_at: "", last_seen_at: "", occurrences: 1, phase: "load", severity: "error", message: "x"}
    ]}));

    expect(items).toEqual([]);
  });

  it("points at nodes that are down, drifted from the master build or need a backend initialised", () => {
    const nodes = [
      inventoryNode("atlas", {role: "master"}),
      inventoryNode("mini", {build_version: "0.6.12"}),
      inventoryNode("forge", {available: false})
    ];
    const snapshots: Record<string, NodeState> = {
      atlas: {node_id: "atlas", active_requests: [], backends: [{id: "vllm", display_name: "vLLM", mode: "vllm", lifecycle_state: "needs_init", loaded_models: []}]}
    };
    const items = attentionItems(sources({nodes, snapshots}));

    expect(items.map(item => [item.key, item.tone])).toEqual([
      ["build-mini", "warning"],
      ["down-forge", "danger"],
      ["backend-atlas-vllm", "warning"]
    ]);
    expect(items[0]?.detail).toBe("The master runs 0.6.14");
    expect(items.every(item => item.target.kind === "node")).toBe(true);
  });
});

describe("recent requests", () => {
  it("leaves model loads out of the request table", () => {
    const event = (eventType: string, startedAt: number): AnalyticsRecentEvent => ({event_type: eventType, started_at: startedAt} as AnalyticsRecentEvent);
    const response = analytics({recent: [event("request", 3), event("model_load", 2), event("request", 1)]});

    expect(recentRequests(response).map(item => item.started_at)).toEqual([3, 1]);
    expect(recentRequests(analytics({enabled: false}))).toEqual([]);
  });
});
