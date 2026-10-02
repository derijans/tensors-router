import { describe, expect, it } from "vitest";
import { bandPath, linePath, paddedPeak, segmentOffsets, stackLayers, tickIndexes } from "../chart-geometry";

describe("chart geometry", () => {
  it("stacks each layer on top of the ones before it and ignores negative values", () => {
    const layers = stackLayers(["text", "image"] as const, {text: [2, 3], image: [1, -4]}, 2);

    expect(layers).toEqual([
      {key: "text", bottom: [0, 0], top: [2, 3]},
      {key: "image", bottom: [2, 3], top: [3, 3]}
    ]);
  });

  it("draws a line from the left edge to the right edge scaled to the peak", () => {
    expect(linePath([0, 10], 10, {width: 100, height: 50})).toBe("M0.00 50.00 L100.00 0.00");
  });

  it("closes a band by walking the bottom edge backwards", () => {
    expect(bandPath([4, 4], [2, 0], 4, {width: 10, height: 4})).toBe("M0.00 0.00 L10.00 0.00 L10.00 4.00 L0.00 2.00 Z");
  });

  it("leaves headroom above the peak and never divides by zero", () => {
    expect(paddedPeak([5, 10])).toBeCloseTo(11);
    expect(paddedPeak([0, 0])).toBe(1);
  });

  it("spreads axis ticks evenly and keeps short series whole", () => {
    expect(tickIndexes(3)).toEqual([0, 1, 2]);
    expect(tickIndexes(25)).toEqual([0, 6, 12, 18, 24]);
    expect(tickIndexes(0)).toEqual([]);
  });

  it("places each segment where the previous one ends", () => {
    expect(segmentOffsets([10, 25, 5])).toEqual([0, 10, 35]);
  });
});
