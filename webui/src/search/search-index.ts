import type { Model, NodeInventory, Recipe, WebUIEntry } from "../types";

export const searchKinds = ["section", "node", "model", "recipe", "webui"] as const;

export type SearchKind = typeof searchKinds[number];

export const searchKindLabels: Record<SearchKind, string> = {
  section: "Sections",
  node: "Nodes",
  model: "Models",
  recipe: "Recipes",
  webui: "WebUIs"
};

export type SearchTarget =
  | {kind: "section"; tab: string}
  | {kind: "node"; nodeID: string}
  | {kind: "model"; modelID: string}
  | {kind: "recipe"; recipeID: string}
  | {kind: "webui"; webuiID: string};

export interface SearchEntry {
  kind: SearchKind;
  label: string;
  detail: string;
  target: SearchTarget;
}

export interface SearchSources {
  sections: readonly {tab: string; label: string}[];
  nodes: readonly NodeInventory[];
  models: readonly Model[];
  recipes: readonly Recipe[];
  webuis: readonly WebUIEntry[];
}

const resultsPerKind = 5;

export function buildSearchIndex(sources: SearchSources): SearchEntry[] {
  return [
    ...sources.sections.map((section): SearchEntry => ({kind: "section", label: section.label, detail: "Go to section", target: {kind: "section", tab: section.tab}})),
    ...sources.nodes.map((node): SearchEntry => ({
      kind: "node",
      label: node.node_id,
      detail: [node.role, node.backend_mode, node.node_url].filter(Boolean).join(" · "),
      target: {kind: "node", nodeID: node.node_id}
    })),
    ...sources.models.map((model): SearchEntry => ({
      kind: "model",
      label: model.public_id || model.local_id,
      detail: [model.node_id, model.backend_mode, model.filename].filter(Boolean).join(" · "),
      target: {kind: "model", modelID: model.public_id || model.local_id}
    })),
    ...sources.recipes.map((recipe): SearchEntry => ({
      kind: "recipe",
      label: recipe.public_id || recipe.id,
      detail: recipe.public_image_id || "Split route",
      target: {kind: "recipe", recipeID: recipe.id}
    })),
    ...sources.webuis.map((webui): SearchEntry => ({
      kind: "webui",
      label: webui.name,
      detail: [webui.node_id, webui.backend, webui.lane].filter(Boolean).join(" · "),
      target: {kind: "webui", webuiID: webui.id}
    }))
  ];
}

export function searchEntries(index: readonly SearchEntry[], query: string): SearchEntry[] {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return [];
  }
  const ranked = index
    .map(entry => ({entry, rank: matchRank(entry, needle)}))
    .filter((candidate): candidate is {entry: SearchEntry; rank: number} => candidate.rank !== null);
  return searchKinds.flatMap(kind => ranked
    .filter(candidate => candidate.entry.kind === kind)
    .sort((left, right) => left.rank - right.rank || left.entry.label.localeCompare(right.entry.label))
    .slice(0, resultsPerKind)
    .map(candidate => candidate.entry));
}

function matchRank(entry: SearchEntry, needle: string): number | null {
  const label = entry.label.toLowerCase();
  if (label.startsWith(needle)) {
    return 0;
  }
  if (label.split(/[\s\-_/.:]+/).some(word => word.startsWith(needle))) {
    return 1;
  }
  if (label.includes(needle)) {
    return 2;
  }
  return entry.detail.toLowerCase().includes(needle) ? 3 : null;
}
