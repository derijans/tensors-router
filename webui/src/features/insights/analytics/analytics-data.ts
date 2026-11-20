import type { AnalyticsQuery, InventoryResponse, SelectChoice } from "../../../types";
import { formatDecimal } from "../../../lib/format-number";
import { uniqueSorted } from "../../../lib/collections";

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

export function promptSpeedPrefix(promptTokensPerSecond: number | undefined): string {
  return promptTokensPerSecond ? `${formatDecimal(promptTokensPerSecond, 0)} tok/s prompt / ` : "";
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

export function choicesWithSelected(options: SelectChoice[], selected: string | undefined): SelectChoice[] {
  if (!selected || options.some(option => option.value === selected)) {
    return options;
  }
  return [...options, {value: selected, label: selected}];
}

export function isAnalyticsPeriod(value: string): value is AnalyticsQuery["period"] {
  return analyticsPeriods.some(period => period.value === value);
}

export function analyticsSectionLabel(section: string): string {
  return analyticsSections.find(option => option.value === section)?.label ?? section;
}
