import { describe, expect, it } from "vitest";
import { analyticsModelChoices, analyticsNodeChoices, chartPoints, formatDurationSeconds, formatMegabytes, formatPercent, normalizedAnalyticsQuery } from "../analytics-data";
import { testInventory, testModel, testNode } from "./factories";

describe("analytics data helpers", () => {
  it("builds sorted unique analytics identifiers from history and live inventory", () => {
    const alpha = testModel("alpha");
    alpha.node_id = "node-b";
    const beta = testModel("beta");
    beta.node_id = "node-a";
    const image = testModel("image-config");
    image.has_llm = false;
    image.has_image = true;
    image.image_id = "image-local";
    const inventory = testInventory([], [], [alpha, beta]);
    inventory.nodes = [
      testNode([beta]),
      {...testNode([alpha, image]), node_id: "node-b"}
    ];

    expect(analyticsNodeChoices(inventory, ["historical-node", "node-b"]).map(choice => choice.value)).toEqual(["", "historical-node", "node-a", "node-b"]);
    expect(analyticsModelChoices(inventory, ["archived", "image-local"]).map(choice => choice.value)).toEqual(["", "alpha", "archived", "beta", "image-local"]);
  });

  it("normalizes empty filter fields away", () => {
    expect(normalizedAnalyticsQuery({
      period: "24h",
      node_id: "",
      model_id: "",
      section: ""
    })).toEqual({period: "24h"});
  });

  it("formats audio durations across units", () => {
    expect(formatDurationSeconds(12.5)).toBe("12.5s");
    expect(formatDurationSeconds(120)).toBe("2m");
    expect(formatDurationSeconds(7200)).toBe("2h");
  });

  it("formats vram values", () => {
    expect(formatMegabytes(1536)).toBe("1,536 MB");
    expect(formatPercent(12.5)).toBe("12.5%");
  });

  it("scales timeline points against the largest request bucket", () => {
    const series = chartPoints([
      {bucket_start: Date.UTC(2026, 5, 1), request_count: 5, input_tokens: 0, output_tokens: 0, total_tokens: 0, image_count: 0, audio_seconds: 0, load_count: 0, vram_peak_mb: 0, vram_peak_percent: 0, vram_total_mb: 0, model_vram_estimate_mb: 0},
      {bucket_start: Date.UTC(2026, 5, 2), request_count: 10, input_tokens: 0, output_tokens: 0, total_tokens: 0, image_count: 0, audio_seconds: 0, load_count: 0, vram_peak_mb: 0, vram_peak_percent: 0, vram_total_mb: 0, model_vram_estimate_mb: 0}
    ], 100, 50);

    expect(series.points).toHaveLength(2);
    expect(series.points[0]?.x).toBe(4);
    expect(series.points[0]?.y).toBe(25);
    expect(series.points[1]?.x).toBe(96);
    expect(series.points[1]?.y).toBe(4);
    expect(series.linePath).toBe("M 4.00 25.00 L 96.00 4.00");
    expect(series.ticks).toEqual([
      {x: 4, label: "Jun 1"},
      {x: 96, label: "Jun 2"}
    ]);
  });
});
