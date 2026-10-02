import { laneAccent } from "./markup-primitives";
import type { AnalyticsTimeline, LaneAccent } from "./types";

export const timelineLanes = ["llm", "image", "voice", "embed", "music", "other"] as const;

export type TimelineLane = typeof timelineLanes[number];

export const timelineLaneLabels: Record<TimelineLane, string> = {
  llm: "Text",
  image: "Image",
  voice: "Voice",
  embed: "Embed",
  music: "Music",
  other: "Other"
};

export interface BucketWindow {
  from: number;
  to: number;
  granularity: string;
}

const bucketMilliseconds: Record<string, number> = {hour: 3_600_000, day: 86_400_000};
const maximumFilledBuckets = 400;

export interface LaneSeries {
  lanes: TimelineLane[];
  values: Record<TimelineLane, number[]>;
  bucketStarts: number[];
}

export function timelineLaneAccent(lane: TimelineLane): LaneAccent {
  return lane === "other" ? "lane-other" : laneAccent(lane);
}

export function laneSeries(timeline: readonly AnalyticsTimeline[], window?: BucketWindow): LaneSeries {
  const bucketStarts = window ? filledBucketStarts(timeline, window) : timeline.map(bucket => bucket.bucket_start);
  const indexByBucketStart = new Map(bucketStarts.map((bucketStart, index) => [bucketStart, index]));
  const values = Object.fromEntries(timelineLanes.map(lane => [lane, bucketStarts.map(() => 0)])) as Record<TimelineLane, number[]>;
  for (const bucket of timeline) {
    const index = indexByBucketStart.get(bucket.bucket_start);
    if (index === undefined) {
      continue;
    }
    let attributed = 0;
    for (const section of bucket.sections ?? []) {
      const lane = timelineLaneFor(section.section);
      values[lane][index] = (values[lane][index] ?? 0) + section.request_count;
      attributed += section.request_count;
    }
    values.other[index] = (values.other[index] ?? 0) + Math.max(0, bucket.request_count - attributed);
  }
  return {
    lanes: timelineLanes.filter(lane => values[lane].some(value => value > 0)),
    values,
    bucketStarts
  };
}

export function filledBucketStarts(timeline: readonly AnalyticsTimeline[], window: BucketWindow): number[] {
  const step = bucketMilliseconds[window.granularity];
  const sparse = timeline.map(bucket => bucket.bucket_start);
  if (!step || window.from <= 0 || window.to < window.from) {
    return sparse;
  }
  const first = Math.floor(window.from / step) * step;
  const last = Math.floor(window.to / step) * step;
  if ((last - first) / step + 1 > maximumFilledBuckets) {
    return sparse;
  }
  const filled = new Set<number>(sparse);
  for (let bucketStart = first; bucketStart <= last; bucketStart += step) {
    filled.add(bucketStart);
  }
  return [...filled].sort((left, right) => left - right);
}

function timelineLaneFor(section: string): TimelineLane {
  return (timelineLanes as readonly string[]).includes(section) && section !== "other" ? section as TimelineLane : "other";
}
