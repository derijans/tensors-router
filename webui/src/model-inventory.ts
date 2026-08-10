import { elements } from "./elements";
import { renderFilesPanel } from "./files-panel";
import { changedNodeSelection, defaultNodeSelection, retainedNodeSelection } from "./model-filter-data";
import { inventoryFiles, inventoryModels } from "./model-inventory-data";
import { renderModelsPanel } from "./models-panel";
import { state } from "./state";
import type { ModelInventorySubtab } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";

export function renderModelInventory(): void {
  const nodes = state.inventory?.nodes ?? [];
  const nodeIDs = nodes.map(node => node.node_id);
  syncNodeFilters(nodeIDs);
  renderNodeFilter(elements.modelsNodeFilter, nodeIDs, state.models.configNodeIDs);
  renderNodeFilter(elements.filesNodeFilter, nodeIDs, state.models.fileNodeIDs);
  elements.modelSearchInput.value = state.models.modelSearch;
  elements.modelEnabledFilter.value = state.models.enabledFilter;
  elements.fileSearchInput.value = state.models.fileSearch;
  elements.fileHashFilter.value = state.models.fileHashFilter;
  renderModelsPanel(inventoryModels(state.inventory?.models ?? [], nodes), nodes);
  renderFilesPanel(inventoryFiles(nodes), nodes);
  renderSubtab();
}

export function activateModelInventorySubtab(subtab: ModelInventorySubtab): void {
  state.models.activeSubtab = subtab;
  renderSubtab();
}

export function updateConfigNodeFilter(values: string[]): void {
  state.models.configNodeIDs = changedNodeSelection(values, state.models.configNodeIDs);
  renderModelInventory();
}

export function updateFileNodeFilter(values: string[]): void {
  state.models.fileNodeIDs = changedNodeSelection(values, state.models.fileNodeIDs);
  renderModelInventory();
}

function syncNodeFilters(nodeIDs: string[]): void {
  const localNodeID = state.inventory?.node_id || "";
  if (!state.models.initialized) {
    state.models.configNodeIDs = defaultNodeSelection(localNodeID, nodeIDs);
    state.models.fileNodeIDs = defaultNodeSelection(localNodeID, nodeIDs);
    state.models.initialized = true;
    return;
  }
  state.models.configNodeIDs = retainedNodeSelection(state.models.configNodeIDs, localNodeID, nodeIDs);
  state.models.fileNodeIDs = retainedNodeSelection(state.models.fileNodeIDs, localNodeID, nodeIDs);
}

function renderNodeFilter(select: HTMLSelectElement, nodeIDs: string[], selected: string[]): void {
  select.innerHTML = `<option value="*"${selected.includes("*") ? " selected" : ""}>All Nodes</option>${nodeIDs.map(nodeID => `<option value="${escapeAttribute(nodeID)}"${selected.includes(nodeID) ? " selected" : ""}>${escapeHTML(nodeID)}</option>`).join("")}`;
}

function renderSubtab(): void {
  document.querySelectorAll<HTMLButtonElement>("[data-model-inventory-subtab]").forEach(button => {
    const active = button.dataset.modelInventorySubtab === state.models.activeSubtab;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", String(active));
  });
  document.querySelectorAll<HTMLElement>("[data-model-inventory-panel]").forEach(panel => panel.classList.toggle("active", panel.dataset.modelInventoryPanel === state.models.activeSubtab));
}
