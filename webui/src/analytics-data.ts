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

export interface ChartBar {
  x: number;
  y: number;
  width: number;
  height: number;
}

export function analyticsNodeChoices(inventory: InventoryResponse | null): SelectChoice[] {
  const nodes = (inventory?.nodes ?? [])
    .map(node => node.node_id)
    .filter(nonEmptyString);
  return [{value: "", label: "All nodes"}, ...uniqueSorted(nodes).map(node => ({value: node, label: node}))];
}

export function analyticsModelChoices(inventory: InventoryResponse | null): SelectChoice[] {
  const models = (inventory?.nodes ?? [])
    .flatMap(node => node.models ?? [])
    .map(model => model.local_id || model.public_id)
    .filter(nonEmptyString);
  return [{value: "", label: "All models"}, ...uniqueSorted(models).map(model => ({value: model, label: model}))];
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

export function chartBars(points: AnalyticsTimeline[], width: number, height: number): ChartBar[] {
  if (points.length === 0 || width <= 0 || height <= 0) {
    return [];
  }
  const max = Math.max(...points.map(point => point.request_count), 1);
  const gap = points.length > 1 ? 3 : 0;
  const barWidth = Math.max(2, (width - gap * (points.length - 1)) / points.length);
  return points.map((point, index) => {
    const barHeight = Math.max(1, (point.request_count / max) * height);
    return {
      x: index * (barWidth + gap),
      y: height - barHeight,
      width: barWidth,
      height: barHeight
    };
  });
}

function uniqueSorted(values: string[]): string[] {
  return Array.from(new Set(values)).sort((left, right) => left.localeCompare(right));
}

function nonEmptyString(value: string | undefined): value is string {
  return Boolean(value?.trim());
}
