import { describe, expect, it } from "vitest";
import { deviceMemoryRows } from "../node-memory-devices-data";
import type { NodeMemory } from "../types";

function memory(overrides: Partial<NodeMemory>): NodeMemory {
  return {kind: "vram", total_mb: 24576, used_mb: 4096, sampled_at_ms: 1, ...overrides};
}

describe("device memory rows", () => {
  it("lists every GPU and then system RAM", () => {
    const rows = deviceMemoryRows(memory({
      devices: [
        {vendor: "nvidia", name: "RTX 3090", source: "nvidia-smi", dedicated_used_mb: 4096, dedicated_total_mb: 24576, usage_known: true},
        {vendor: "amd", name: "Radeon RX 7900", source: "dxgi+pdh", dedicated_used_mb: 2048, dedicated_total_mb: 20480, shared_used_mb: 512, usage_known: true}
      ],
      system_ram: {used_mb: 16384, total_mb: 65536}
    }));

    expect(rows.map(row => row.name)).toEqual(["RTX 3090", "Radeon RX 7900", "System RAM"]);
    expect(rows[0]?.usedPercent).toBeCloseTo(16.67, 1);
    expect(rows[1]?.detail).toContain("0.5 GB shared");
    expect(rows[2]?.usedPercent).toBe(25);
  });

  it("shows a GPU whose usage is unknown without inventing a usage bar", () => {
    const rows = deviceMemoryRows(memory({
      devices: [{vendor: "intel", name: "Arc A770", source: "dxgi", dedicated_used_mb: 0, dedicated_total_mb: 16288, usage_known: false}]
    }));

    expect(rows).toHaveLength(1);
    expect(rows[0]?.usedPercent).toBeNull();
    expect(rows[0]?.detail).toContain("usage not reported");
  });

  it("adds nothing for a RAM-only node that lists no GPUs", () => {
    expect(deviceMemoryRows(memory({kind: "ram", system_ram: {used_mb: 1, total_mb: 2}}))).toEqual([]);
  });

  it("skips devices with no dedicated memory", () => {
    const rows = deviceMemoryRows(memory({
      devices: [{vendor: "virtual", name: "Basic Render", source: "dxgi", dedicated_used_mb: 0, dedicated_total_mb: 0, usage_known: false}]
    }));
    expect(rows.map(row => row.name)).toEqual([]);
  });
});
