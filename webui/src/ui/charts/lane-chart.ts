import type { SafeHTML } from "../../lib/safe-html";
import type { StackedSeries } from "../../types";
import { stackedAreaChart } from "./chart-markup";

export function renderLaneChart(series: StackedSeries, granularity: string): SafeHTML {
  return stackedAreaChart(series, bucketStart => formatBucketTick(bucketStart, granularity), "Requests by lane over time");
}

export function formatBucketTick(bucketStart: number, granularity: string): string {
  const date = new Date(bucketStart);
  if (granularity === "hour") {
    return date.toLocaleTimeString("en-GB", {hour: "2-digit", minute: "2-digit"});
  }
  return date.toLocaleDateString("en-US", {month: "short", day: "numeric"});
}
