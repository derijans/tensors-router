import { inventoryScanNotices } from "./inventory-scan-notices";
import { SafeHTML, emptyHTML, html, setHTML } from "./safe-html";
import { benchmarkCompactLabel } from "./benchmark-data";
import { elements } from "./elements";
import { filterInventoryModels, modelBackends, modelCapabilities } from "./model-inventory-data";
import { linkCountsForModel, routingButtonLabel } from "./routing-links-data";
import { state } from "./state";
import type { Model, NodeInventory, RoutingEndpoint, RoutingLane, Tone } from "./types";
import { badge, laneAccent, optionElement } from "./markup-primitives";
import { capabilities, optionSummary } from "./utils";
import { modelColumns, visibleModelColumns, type ModelColumnKey } from "./model-columns";

export function renderModelsPanel(models: Model[], nodes: NodeInventory[]): void {
  renderSelect(elements.modelBackendFilter, "All backends", modelBackends(models), state.models.backendFilter);
  renderSelect(elements.modelCapabilityFilter, "All capabilities", modelCapabilities(models), state.models.capabilityFilter);
  const filtered = filterInventoryModels(models, {
    query: state.models.modelSearch,
    nodeIDs: state.models.configNodeIDs,
    enabled: state.models.enabledFilter,
    backend: state.models.backendFilter,
    capability: state.models.capabilityFilter
  });
  elements.modelsRowCount.textContent = `${filtered.length} of ${models.length} models`;
  setHTML(elements.modelsScanNotices, inventoryScanNotices(nodes));
  const columns = visibleModelColumns(state.models.columnChoices, state.models.roomForEveryColumn);
  setHTML(elements.modelColumnsList, columnChoicesMarkup(columns));
  setHTML(elements.modelsTableHead, html`<tr>${columns.map(columnHeader)}</tr>`);
  setHTML(elements.modelsTable, filtered.length > 0
    ? html`${filtered.map(model => modelRow(model, columns))}`
    : html`<tr><td class="empty-state" colspan="${columns.length}">No models match the current filters.</td></tr>`);
}

function columnHeader(column: ModelColumnKey): SafeHTML {
  return html`<th class="column-${column}">${columnLabel(column)}</th>`;
}

function columnLabel(column: ModelColumnKey): string {
  return modelColumns.find(definition => definition.key === column)?.label ?? column;
}

function columnChoicesMarkup(visible: readonly ModelColumnKey[]): SafeHTML {
  return html`${modelColumns.map(column => columnChoice(column, visible.includes(column.key)))}`;
}

function columnChoice(column: typeof modelColumns[number], checked: boolean): SafeHTML {
  return html`<label class="toggle-row"><input type="checkbox" data-model-column="${column.key}"${checked ? " checked" : ""}><span>${column.label}</span></label>`;
}

function modelRow(model: Model, columns: readonly ModelColumnKey[]): SafeHTML {
  return html`<tr class="${model.disabled ? "row-disabled" : ""}">${columns.map(column => modelCellElement(model, column))}</tr>`;
}

function modelCellElement(model: Model, column: ModelColumnKey): SafeHTML {
  return html`<td class="column-${column}">${modelCell(model, column)}</td>`;
}


function modelCell(model: Model, column: ModelColumnKey): SafeHTML | string {
  const enabled = !model.disabled;
  const operationGroup = `model-state-${model.node_id}-${model.local_id}`;
  switch (column) {
    case "id":
      return html`<div class="model-cell"><strong>${model.public_id || model.local_id}</strong><small>${model.filename}</small></div>`;
    case "node":
      return model.node_id || "";
    case "enabled":
      return html`<label class="model-enabled-switch" title="${enabled ? "Disable model" : "Enable model"}"><input type="checkbox" ${enabled ? "checked" : ""} data-operation-group="${operationGroup}" data-model-enabled-node="${model.node_id}" data-model-enabled-id="${model.local_id}"><span aria-hidden="true"></span><span class="sr-only">${enabled ? "Enabled" : "Disabled"}</span></label>`;
    case "backend":
      return model.backend_mode || "";
    case "capabilities":
      return html`<div class="badge-row">${capabilityBadges(model)}</div>`;
    case "options":
      return optionSummary(model.options);
    case "benchmark":
      return benchmarkCompactLabel(model);
    case "available":
      return modelAssetAvailability(model);
    case "routing":
      return routingCell(model);
    case "separate":
      return separateCell(model, operationGroup);
    case "actions":
      return html`<button type="button" data-operation-group="${operationGroup}" data-load-config="${model.public_id || model.local_id}" ${enabled ? "" : "disabled data-unavailable"}>Load</button>`;
  }
}

function routingCell(model: Model): SafeHTML {
  if (!model.node_id) {
    return emptyHTML;
  }
  const buttons: SafeHTML[] = [];
  if (model.image_id) {
    buttons.push(routingButton("image", {node_id: model.node_id, model_id: model.image_id}));
  }
  if (model.has_llm) {
    buttons.push(routingButton("text", {node_id: model.node_id, model_id: model.local_id}));
  }
  return html`<div class="cell-stack">${buttons}</div>`;
}

function routingButton(lane: RoutingLane, endpoint: RoutingEndpoint): SafeHTML {
  const label = routingButtonLabel(lane, linkCountsForModel(state.routingLinks[lane], endpoint));
  return html`<button type="button" data-routing-lane="${lane}" data-routing-node="${endpoint.node_id}" data-routing-model="${endpoint.model_id}">${label}</button>`;
}

// Only kobold and llama configs can run in a separate process; vLLM rows leave the
// cell empty rather than offer a control that would do nothing.
function separateCell(model: Model, operationGroup: string): SafeHTML {
  if (!model.node_id || model.backend_mode === "vllm") {
    return emptyHTML;
  }
  return html`<button type="button" data-operation-group="${operationGroup}" data-separate-node="${model.node_id}" data-separate-id="${model.local_id}">Separate</button>`;
}

function modelAssetAvailability(model: Model): SafeHTML {
  let label = model.available ? "ready" : "unavailable";
  let assetState = model.available ? "ready" : "failed";
  if (model.asset_state === "unresolved") {
    label = `unresolved (${model.unresolved_fields ?? 0})`;
    assetState = "unresolved";
  }
  if (model.asset_state === "failed" || model.asset_state === "resolving") {
    label = model.asset_failure ? `${model.asset_state}: ${model.asset_failure}` : model.asset_state;
    assetState = model.asset_state;
  }
  return badge(label, assetTone(assetState));
}

function assetTone(assetState: string): Tone {
  switch (assetState) {
    case "ready":
      return "success";
    case "unresolved":
      return "warning";
    case "resolving":
      return "info";
    default:
      return "danger";
  }
}

function capabilityBadges(model: Model): SafeHTML {
  const values = capabilities(model);
  if (values.length === 0) {
    return html`<span class="muted">none</span>`;
  }
  return html`${values.map(value => badge(value, value === "multimodal" ? "neutral" : laneAccent(value)))}`;
}

function renderSelect(select: HTMLSelectElement, allLabel: string, values: string[], selected: string): void {
  setHTML(select, html`<option value="">${allLabel}</option>${values.map(value => optionElement(value, value, value === selected))}`);
}
