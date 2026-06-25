import { describe, expect, it } from "vitest";
import { analyticsModelChoices, analyticsNodeChoices, chartBars, formatDurationSeconds, normalizedAnalyticsQuery } from "../analytics-data";
import { testInventory, testModel, testNode } from "./factories";

describe("analytics data helpers", () => {
  it("builds sorted unique node and model choices", () => {
    const alpha = testModel("alpha");
    alpha.node_id = "node-b";
    const beta = testModel("beta");
    beta.node_id = "node-a";
    const inventory = testInventory([], [], [alpha, beta]);
    inventory.nodes = [
      testNode([beta]),
      {...testNode([alpha]), node_id: "node-b"}
    ];

    expect(analyticsNodeChoices(inventory).map(choice => choice.value)).toEqual(["", "node-a", "node-b"]);
    expect(analyticsModelChoices(inventory).map(choice => choice.value)).toEqual(["", "alpha", "beta"]);
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

  it("scales timeline bars against the largest request bucket", () => {
    const bars = chartBars([
      {bucket_start: 1, request_count: 5, input_tokens: 0, output_tokens: 0, total_tokens: 0, image_count: 0, audio_seconds: 0},
      {bucket_start: 2, request_count: 10, input_tokens: 0, output_tokens: 0, total_tokens: 0, image_count: 0, audio_seconds: 0}
    ], 100, 50);

    expect(bars).toHaveLength(2);
    expect(bars[0]?.height).toBe(25);
    expect(bars[1]?.height).toBe(50);
  });
});
