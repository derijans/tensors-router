import { inventoryScanNotices } from "../nodes/inventory-scan-notices-view";
import { SafeHTML, emptyHTML, html, setHTML } from "../../lib/safe-html";
import { benchmarkCompactLabel } from "../insights/benchmarks/benchmark-data";
import { filterInventoryModels, modelBackends, presentModelCapabilities } from "./model-inventory-data";
import { linkCountsForModel, routingButtonLabel } from "./routing-links/routing-links-data";
import { state } from "../../state/app-state";
import type { Model, NodeInventory, RoutingEndpoint, RoutingLane, Tone } from "../../types";
import { badge, badgeRow } from "../../ui/badge";
import { modelColumns, visibleModelColumns, type ModelColumnKey } from "./model-columns-data";
import { memoryFitFor } from "./memory-fit/memory-fit-data";
import { renderMemoryFitBadge } from "./memory-fit/memory-fit-badge-view";
import { laneAccent } from "../../domain/lanes";
import { fillSelectWithAll } from "../../ui/select";
import { modelCapabilityNames } from "../../domain/model-capabilities";
import { optionSummary } from "../../domain/option-values";
import { emptyTableRow } from "../../ui/empty-state";
import { mutedText } from "../../ui/text";
import { checkbox, labelledToggle } from "../../ui/form";
import { modelsElements } from "./models-elements";
import { lookupOr } from "../../lib/collections";
import { button } from "../../ui/buttons";

export function renderModelsPanel(models: Model[], nodes: NodeInventory[]): void {
  fillSelectWithAll(modelsElements.modelBackendFilter, "All backends", modelBackends(models), state.models.backendFilter);
  fillSelectWithAll(modelsElements.modelCapabilityFilter, "All capabilities", presentModelCapabilities(models), state.models.capabilityFilter);
  const filtered = filterInventoryModels(models, {
    query: state.models.modelSearch,
    nodeIDs: state.models.configNodeIDs,
    enabled: state.models.enabledFilter,
    backend: state.models.backendFilter,
    capability: state.models.capabilityFilter
  });
  modelsElements.modelsRowCount.textContent = `${filtered.length} of ${models.length} models`;
  setHTML(modelsElements.modelsScanNotices, inventoryScanNotices(nodes));
  const columns = visibleModelColumns(state.models.columnChoices, state.models.roomForEveryColumn);
  setHTML(modelsElements.modelColumnsList, columnChoicesMarkup(columns));
  setHTML(modelsElements.modelsTableHead, html`<tr>${columns.map(columnHeader)}</tr>`);
  setHTML(modelsElements.modelsTable, filtered.length > 0
    ? html`${filtered.map(model => modelRow(model, columns))}`
    : emptyTableRow("No models match the current filters.", columns.length));
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
  return labelledToggle({"data-model-column": column.key}, checked, column.label);
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
      return html`<label class="model-enabled-switch" title="${enabled ? "Disable model" : "Enable model"}">${checkbox({"data-operation-group": operationGroup, "data-model-enabled-node": model.node_id, "data-model-enabled-id": model.local_id}, enabled)}<span aria-hidden="true"></span><span class="sr-only">${enabled ? "Enabled" : "Disabled"}</span></label>`;
    case "backend":
      return model.backend_mode || "";
    case "capabilities":
      return badgeRow(capabilityBadges(model));
    case "options":
      return optionSummary(model.options);
    case "benchmark":
      return benchmarkCompactLabel(model);
    case "fit":
      return renderMemoryFitBadge(memoryFitFor(state.models.memoryFit, model.node_id, model.local_id));
    case "available":
      return modelAssetAvailability(model);
    case "routing":
      return routingCell(model);
    case "separate":
      return separateCell(model, operationGroup);
    case "actions":
      return button("Load", {"data-operation-group": operationGroup, "data-load-config": model.public_id || model.local_id, disabled: !enabled, "data-unavailable": !enabled});
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

const assetTones: Record<string, Tone> = {
  ready: "success",
  unresolved: "warning",
  resolving: "info"
};

function assetTone(assetState: string): Tone {
  return lookupOr(assetTones, assetState, "danger");
}

function capabilityBadges(model: Model): SafeHTML {
  const values = modelCapabilityNames(model);
  if (values.length === 0) {
    return mutedText("none");
  }
  return html`${values.map(value => badge(value, value === "multimodal" ? "neutral" : laneAccent(value)))}`;
}
