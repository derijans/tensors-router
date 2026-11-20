import { cloneOptions } from "../../../domain/option-values";
import { state } from "../../../state/app-state";
import type { Model, Options, SimpleCookMode } from "../../../types";
import { cookElements } from "../cook-elements";
import { clearConversionScope } from "../conversions";
import { suggestedConfigID } from "./simple-cook-data";
import { selectedConfig, selectedNode } from "./simple-cook-selection";

export function loadSimpleConfig(model: Model | null): void {
  state.simpleCook.mode = "edit";
  state.simpleCook.configID = model?.local_id || "";
  state.simpleCook.fields = cloneOptions(model?.options ?? {});
  state.simpleCook.cleanFields = cloneOptions(model?.options ?? {});
  state.simpleCook.sidebar = null;
  state.simpleCook.openSections = [];
  cookElements.cookIdInput.value = model?.local_id || suggestedConfigID(selectedNode(), "new-config");
  state.simpleCook.cleanID = cookElements.cookIdInput.value.trim();
  clearConversionScope("quick");
}

export function startUnsavedDraft(mode: Exclude<SimpleCookMode, "edit">, fields: Options, idFallback: string): void {
  state.simpleCook.mode = mode;
  state.simpleCook.configID = "";
  state.simpleCook.fields = fields;
  state.simpleCook.cleanFields = {};
  state.simpleCook.cleanID = "";
  state.simpleCook.openSections = [];
  clearConversionScope("quick");
  cookElements.cookIdInput.value = suggestedConfigID(selectedNode(), idFallback);
}

export function syncSimpleCookSelection(): void {
  const nodes = state.inventory?.nodes ?? [];
  if (nodes.length === 0) {
    state.simpleCook.nodeID = "";
    state.simpleCook.configID = "";
    state.simpleCook.fields = {};
    state.simpleCook.cleanFields = {};
    state.simpleCook.openSections = [];
    return;
  }
  if (!nodes.some(node => node.node_id === state.simpleCook.nodeID)) {
    const firstNode = nodes[0];
    state.simpleCook.nodeID = firstNode?.node_id ?? "";
    loadSimpleConfig((firstNode?.models ?? [])[0] ?? null);
    return;
  }
  if (state.simpleCook.mode !== "edit") {
    return;
  }
  const currentNode = selectedNode();
  if (!selectedConfig() && (currentNode?.models ?? []).length > 0) {
    loadSimpleConfig((currentNode?.models ?? [])[0] ?? null);
  }
}
