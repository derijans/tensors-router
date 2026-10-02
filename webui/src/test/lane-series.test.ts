import { describe, expect, it } from "vitest";
import { laneSeries } from "../lane-series";
import type { AnalyticsTimeline } from "../types";

function bucket(bucketStart: number, requestCount: number, sections?: AnalyticsTimeline["sections"]): AnalyticsTimeline {
  const value: AnalyticsTimeline = {
    bucket_start: bucketStart,
    request_count: requestCount,
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: 0,
    image_count: 0,
    embedding_count: 0,
    audio_seconds: 0,
    load_count: 0,
    vram_peak_mb: 0,
    vram_peak_percent: 0,
    vram_total_mb: 0,
    model_vram_estimate_mb: 0
  };
  if (sections) {
    value.sections = sections;
  }
  return value;
}

describe("lane series", () => {
  it("splits each bucket by lane and keeps only lanes that saw traffic", () => {
    const series = laneSeries([
      bucket(1, 5, [{section: "llm", request_count: 3}, {section: "image", request_count: 2}]),
      bucket(2, 4, [{section: "llm", request_count: 4}])
    ]);

    expect(series.lanes).toEqual(["llm", "image"]);
    expect(series.values.llm).toEqual([3, 4]);
    expect(series.values.image).toEqual([2, 0]);
    expect(series.bucketStarts).toEqual([1, 2]);
  });

  it("counts requests from nodes that report no lane breakdown as other", () => {
    const series = laneSeries([
      bucket(1, 6, [{section: "llm", request_count: 2}]),
      bucket(2, 3)
    ]);

    expect(series.lanes).toEqual(["llm", "other"]);
    expect(series.values.other).toEqual([4, 3]);
  });

  it("fills the whole period with empty buckets so a quiet hour still takes its place on the axis", () => {
    const hour = 3_600_000;
    const series = laneSeries([bucket(2 * hour, 4, [{section: "llm", request_count: 4}])], {from: hour + 1000, to: 3 * hour + 5, granularity: "hour"});

    expect(series.bucketStarts).toEqual([hour, 2 * hour, 3 * hour]);
    expect(series.values.llm).toEqual([0, 4, 0]);
  });

  it("folds sections it does not know into other", () => {
    const series = laneSeries([bucket(1, 2, [{section: "video", request_count: 2}])]);

    expect(series.lanes).toEqual(["other"]);
    expect(series.values.other).toEqual([2]);
  });
});
