import { describe, expect, it } from "vitest";
import { formatGigabytes, nodeMemorySummary } from "../node-memory-data";
import type { NodeState, NodeStateModelRow } from "../types";

function snapshot(usedMB: number, rows: NodeStateModelRow[]): NodeState {
  return {
    node_id: "node-a",
    active_requests: [],
    memory: {kind: "vram", total_mb: 1000, used_mb: usedMB, sampled_at_ms: 1},
    backends: [{id: "koboldcpp", display_name: "KoboldCpp", mode: "kobold", lifecycle_state: "ready", loaded_models: rows}]
  };
}

function row(modelID: string, lane: string, memoryMB: number, borrowed = false): NodeStateModelRow {
  return {model_id: modelID, lane, runtime_id: `${lane}-0`, generation: 1, memory_estimate_mb: memoryMB, borrowed};
}

describe("node memory summary", () => {
  it("gives each loaded runtime its share and leaves the rest of the used memory as other", () => {
    const summary = nodeMemorySummary(snapshot(600, [row("qwen", "text", 300), row("flux", "image", 200, true)]));

    expect(summary?.kind).toBe("vram");
    expect(summary?.segments.map(segment => [segment.accent, segment.percent])).toEqual([
      ["lane-text", 30],
      ["lane-lent", 20],
      ["lane-other", 10]
    ]);
  });

  it("scales runtime estimates down when together they exceed what the node reports as used", () => {
    const summary = nodeMemorySummary(snapshot(400, [row("qwen", "text", 400), row("flux", "image", 400)]));

    expect(summary?.segments.map(segment => segment.percent)).toEqual([20, 20]);
  });

  it("has nothing to draw when the node reports no memory", () => {
    const state = snapshot(100, []);
    delete state.memory;

    expect(nodeMemorySummary(state)).toBeNull();
    expect(nodeMemorySummary(null)).toBeNull();
  });

  it("formats megabytes as gigabytes", () => {
    expect(formatGigabytes(12288)).toBe("12.0 GB");
    expect(formatGigabytes(196608)).toBe("192 GB");
  });
});
