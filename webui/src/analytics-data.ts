import type { AnalyticsQuery, AnalyticsTimeline, InventoryResponse, SelectChoice } from "./types";

export const analyticsPeriods: SelectChoice[] = [
  {value: "24h", label: "Last 24 hours"},
  {value: "7d", label: "Last 7 days"},
  {value: "30d", label: "Last 30 days"},
  {value: "90d", label: "Last 90 days"},
  {value: "all", label: "All time"}
];

export const analyticsSections: SelectChoice[] = [
  {value: "", label: "All sections"},
  {value: "llm", label: "LLM"},
  {value: "embed", label: "Embeddings"},
  {value: "image", label: "Images"},
  {value: "voice", label: "Voice"},
  {value: "music", label: "Music"}
];

export interface ChartPoint {
  x: number;
  y: number;
  radius: number;
}

export interface ChartTick {
  x: number;
  label: string;
}

export interface ChartPointSeries {
  points: ChartPoint[];
  linePath: string;
  ticks: ChartTick[];
}

export function analyticsNodeChoices(inventory: InventoryResponse | null, historicalNodeIDs: string[] = []): SelectChoice[] {
  const liveNodeIDs = (inventory?.nodes ?? [])
    .map(node => node.node_id)
    .filter(nonEmptyString);
  return [{value: "", label: "All nodes"}, ...uniqueSorted([...historicalNodeIDs, ...liveNodeIDs]).map(node => ({value: node, label: node}))];
}

export function analyticsModelChoices(inventory: InventoryResponse | null, historicalModelIDs: string[] = []): SelectChoice[] {
  const liveModelIDs = (inventory?.nodes ?? [])
    .flatMap(node => node.models ?? [])
    .flatMap(model => analyticsModelIDs(model))
    .filter(nonEmptyString);
  return [{value: "", label: "All models"}, ...uniqueSorted([...historicalModelIDs, ...liveModelIDs]).map(model => ({value: model, label: model}))];
}

export function normalizedAnalyticsQuery(query: AnalyticsQuery): AnalyticsQuery {
  const normalized: AnalyticsQuery = {period: query.period || "24h"};
  if (query.node_id) {
    normalized.node_id = query.node_id;
  }
  if (query.model_id) {
    normalized.model_id = query.model_id;
  }
  if (query.section) {
    normalized.section = query.section;
  }
  return normalized;
}

export function formatCount(value: number | undefined): string {
  const safe = Number.isFinite(value ?? 0) ? value ?? 0 : 0;
  return Math.round(safe).toLocaleString("en-US");
}

export function formatDecimal(value: number | undefined, digits = 1): string {
  const safe = Number.isFinite(value ?? 0) ? value ?? 0 : 0;
  if (Number.isInteger(safe)) {
    return safe.toLocaleString("en-US");
  }
  return safe.toLocaleString("en-US", {
    maximumFractionDigits: digits,
    minimumFractionDigits: safe > 0 && safe < 10 ? digits : 0
  });
}

export function formatDurationSeconds(value: number | undefined): string {
  const seconds = Number.isFinite(value ?? 0) ? value ?? 0 : 0;
  if (seconds < 60) {
    return `${formatDecimal(seconds, 1)}s`;
  }
  const minutes = seconds / 60;
  if (minutes < 60) {
    return `${formatDecimal(minutes, 1)}m`;
  }
  return `${formatDecimal(minutes / 60, 1)}h`;
}

export function formatMegabytes(value: number | undefined): string {
  return `${formatCount(value)} MB`;
}

export function formatPercent(value: number | undefined): string {
  return `${formatDecimal(value, 1)}%`;
}

export function chartPoints(timeline: AnalyticsTimeline[], width: number, height: number): ChartPointSeries {
  const empty: ChartPointSeries = {points: [], linePath: "", ticks: []};
  if (timeline.length === 0 || width <= 0 || height <= 0) {
    return empty;
  }
  const max = Math.max(...timeline.map(point => point.request_count), 1);
  const radius = 4;
  const xSpan = Math.max(0, width - radius * 2);
  const ySpan = Math.max(0, height - radius * 2);
  const lastIndex = timeline.length - 1;
  const points = timeline.map((point, index) => {
    const xRatio = lastIndex === 0 ? 0.5 : index / lastIndex;
    return {
      x: radius + xRatio * xSpan,
      y: radius + (1 - point.request_count / max) * ySpan,
      radius
    };
  });
  return {
    points,
    linePath: points.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(" "),
    ticks: chartTicks(timeline, points)
  };
}

function chartTicks(timeline: AnalyticsTimeline[], points: ChartPoint[]): ChartTick[] {
  if (timeline.length === 0 || points.length === 0) {
    return [];
  }
  const lastIndex = timeline.length - 1;
  const indexes = lastIndex <= 3
    ? timeline.map((_, index) => index)
    : [0, Math.round(lastIndex / 3), Math.round((lastIndex * 2) / 3), lastIndex];
  return Array.from(new Set(indexes)).map(index => ({
    x: points[index]?.x ?? 0,
    label: formatTickDate(timeline[index]?.bucket_start)
  }));
}

function formatTickDate(value: number | undefined): string {
  if (!value) {
    return "";
  }
  return new Date(value).toLocaleDateString("en-US", {month: "short", day: "numeric"});
}

function uniqueSorted(values: string[]): string[] {
  return Array.from(new Set(values)).sort((left, right) => left.localeCompare(right));
}

function analyticsModelIDs(model: InventoryResponse["nodes"][number]["models"][number]): string[] {
  const modelIDs: string[] = [];
  if (model.has_llm || model.has_embeddings || model.has_multimodal || model.has_voice || model.has_music) {
    modelIDs.push(model.local_id || model.public_id);
  }
  if (model.has_image) {
    modelIDs.push(model.image_id || model.local_id || model.public_id);
  }
  return modelIDs;
}

function nonEmptyString(value: string | undefined): value is string {
  return Boolean(value?.trim());
}
