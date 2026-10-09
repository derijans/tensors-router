import { optionElement } from "./markup-primitives";
import { html, setHTML } from "./safe-html";
import { elements } from "./elements";
import { renderFilesPanel } from "./files-panel";
import { changedNodeSelection, defaultNodeSelection, retainedNodeSelection } from "./model-filter-data";
import { inventoryFiles, inventoryModels } from "./model-inventory-data";
import { renderModelsPanel } from "./models-panel";
import { state } from "./state";
import type { ModelInventorySubtab } from "./types";

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
  if (!state.models.initialized) {
    state.models.configNodeIDs = defaultNodeSelection();
    state.models.fileNodeIDs = defaultNodeSelection();
    state.models.initialized = true;
    return;
  }
  state.models.configNodeIDs = retainedNodeSelection(state.models.configNodeIDs, nodeIDs);
  state.models.fileNodeIDs = retainedNodeSelection(state.models.fileNodeIDs, nodeIDs);
}

function renderNodeFilter(select: HTMLSelectElement, nodeIDs: string[], selected: string[]): void {
  setHTML(select, html`<option value="*"${selected.includes("*") ? " selected" : ""}>All Nodes</option>${nodeIDs.map(nodeID => optionElement(nodeID, nodeID, selected.includes(nodeID)))}`);
}

function renderSubtab(): void {
  document.querySelectorAll<HTMLButtonElement>("[data-model-inventory-subtab]").forEach(button => {
    const active = button.dataset.modelInventorySubtab === state.models.activeSubtab;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", String(active));
  });
  document.querySelectorAll<HTMLElement>("[data-model-inventory-panel]").forEach(panel => panel.classList.toggle("active", panel.dataset.modelInventoryPanel === state.models.activeSubtab));
}
