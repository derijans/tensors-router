import type { WebUICompatibleModel, WebUIEntry } from "./types";

export type WebUIOpenReason = "disabled" | "needs_model" | "";

export interface WebUIOpenStatus {
  openable: boolean;
  reason: WebUIOpenReason;
}

export interface WebUIGroup {
  nodeID: string;
  entries: WebUIEntry[];
}

export interface WebUIDialogData {
  title: string;
  message: string;
  canEnable: boolean;
  canLoad: boolean;
  models: WebUICompatibleModel[];
}

export function filteredWebUIEntries(entries: WebUIEntry[], query: string): WebUIEntry[] {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return entries;
  }
  return entries.filter(entry => [
    entry.name,
    entry.backend,
    entry.backend_mode,
    entry.lane,
    entry.node_id,
    entry.url,
    ...entry.compatible_models.map(model => model.id)
  ].join(" ").toLowerCase().includes(needle));
}

export function groupWebUIs(entries: WebUIEntry[]): WebUIGroup[] {
  const groups = new Map<string, WebUIEntry[]>();
  for (const entry of entries) {
    const nodeID = entry.node_id || "local";
    groups.set(nodeID, [...(groups.get(nodeID) ?? []), entry]);
  }
  return Array.from(groups.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([nodeID, groupEntries]) => ({
      nodeID,
      entries: [...groupEntries].sort((left, right) => left.name.localeCompare(right.name))
    }));
}

export function webUIOpenStatus(entry: WebUIEntry): WebUIOpenStatus {
  if (!entry.enabled) {
    return {openable: false, reason: "disabled"};
  }
  if (entry.requires_loaded_model && !entry.can_open_without_model && !entry.active) {
    return {openable: false, reason: "needs_model"};
  }
  return {openable: true, reason: ""};
}

export function webUIDialogData(entry: WebUIEntry): WebUIDialogData {
  const status = webUIOpenStatus(entry);
  return {
    title: entry.name,
    message: dialogMessage(status.reason),
    canEnable: !entry.enabled,
    canLoad: entry.compatible_models.length > 0,
    models: webUIModelChoices(entry)
  };
}

export function webUIModelChoices(entry: WebUIEntry): WebUICompatibleModel[] {
  return [...entry.compatible_models].sort((left, right) => {
    if (left.active !== right.active) {
      return left.active ? -1 : 1;
    }
    return left.id.localeCompare(right.id);
  });
}

function dialogMessage(reason: WebUIOpenReason): string {
  switch (reason) {
    case "disabled":
      return "Enable this WebUI before opening.";
    case "needs_model":
      return "Load a compatible model before opening.";
    default:
      return "Ready to open.";
  }
}
