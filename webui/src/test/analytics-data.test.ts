import { describe, expect, it } from "vitest";
import { analyticsModelChoices, analyticsNodeChoices, formatDurationSeconds, formatMegabytes, formatPercent, normalizedAnalyticsQuery } from "../analytics-data";
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
});
