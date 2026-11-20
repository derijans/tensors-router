import { describe, expect, it } from "vitest";
import { memoryFitBadge, memoryFitFor } from "../features/models/memory-fit/memory-fit-data";
import type { ConfigMemoryFit, NodeMemoryFit } from "../types";

function fit(overrides: Partial<ConfigMemoryFit>): ConfigMemoryFit {
  return {
    model_id: "qwen",
    config_name: "qwen.kcpps",
    level: "green",
    tier: "measured",
    vram: {low_mb: 6144, high_mb: 6144},
    ram: {low_mb: 512, high_mb: 1024},
    gpu_free_mb: 16384,
    ram_free_mb: 32768,
    ram_only: false,
    ...overrides
  };
}

describe("memory fit badge", () => {
  it("names each level and maps it to its own class", () => {
    const levels = ["green", "yellow", "light_amber", "deep_amber", "red", "unknown"] as const;
    const badges = levels.map(level => memoryFitBadge(fit({level})));
    expect(badges.map(badge => badge.label)).toEqual(["fits", "tight", "small spill", "large spill", "won't fit", "not learned"]);
    expect(badges.map(badge => badge.className)).toEqual([
      "fit-badge fit-green", "fit-badge fit-yellow", "fit-badge fit-light-amber", "fit-badge fit-deep-amber", "fit-badge fit-red", "fit-badge fit-unknown"
    ]);
  });

  it("explains spill, multi-GPU and that the load still runs", () => {
    const tooltip = memoryFitBadge(fit({level: "deep_amber", spill_mb: 2048, multi_gpu: true, tier: "context_scaled"})).tooltip;
    expect(tooltip).toContain("scaled from another context size");
    expect(tooltip).toContain("2.0 GB may spill");
    expect(tooltip).toContain("all GPUs together");
    expect(tooltip).toContain("never blocked");
  });

  it("talks about memory, not VRAM, for a RAM-only config", () => {
    const tooltip = memoryFitBadge(fit({ram_only: true})).tooltip;
    expect(tooltip).toContain("memory and");
    expect(tooltip).not.toContain("Free VRAM");
  });

  it("shows why a node cannot judge fit", () => {
    expect(memoryFitBadge(fit({level: "unknown", unavailable: "this node reports no memory readings"})).tooltip).toBe("this node reports no memory readings");
  });
});

describe("memory fit lookup", () => {
  it("finds the config on its own node only", () => {
    const fits: Record<string, NodeMemoryFit> = {"node-a": {node_id: "node-a", configs: [fit({})]}};
    expect(memoryFitFor(fits, "node-a", "qwen")?.config_name).toBe("qwen.kcpps");
    expect(memoryFitFor(fits, "node-b", "qwen")).toBeNull();
  });
});
