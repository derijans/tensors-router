import { benchmarkCompactLabel } from "./benchmark-data";
import { elements } from "./elements";
import { filterInventoryModels, modelBackends, modelCapabilities } from "./model-inventory-data";
import { peerCountForModel, routingButtonLabel } from "./routing-groups-data";
import { state } from "./state";
import type { Model, NodeInventory } from "./types";
import { capabilities, escapeAttribute, escapeHTML, optionSummary } from "./utils";

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
  elements.modelsScanNotices.innerHTML = scanNotices(nodes);
  elements.modelsTable.innerHTML = filtered.length > 0 ? filtered.map(modelRow).join("") : `<tr><td class="inventory-empty" colspan="10">No models match the current filters.</td></tr>`;
}

function modelRow(model: Model): string {
  const enabled = !model.disabled;
  const operationGroup = `model-state-${model.node_id}-${model.local_id}`;
  return `
    <tr class="${enabled ? "" : "inventory-row-disabled"}">
      <td title="${escapeAttribute(model.filename)}">${escapeHTML(model.public_id || model.local_id)}</td>
      <td>${escapeHTML(model.node_id || "")}</td>
      <td><label class="model-enabled-switch" title="${enabled ? "Disable model" : "Enable model"}"><input type="checkbox" ${enabled ? "checked" : ""} data-operation-group="${escapeAttribute(operationGroup)}" data-model-enabled-node="${escapeAttribute(model.node_id)}" data-model-enabled-id="${escapeAttribute(model.local_id)}"><span aria-hidden="true"></span><span class="sr-only">${enabled ? "Enabled" : "Disabled"}</span></label></td>
      <td>${escapeHTML(model.backend_mode || "")}</td>
      <td>${escapeHTML(capabilities(model))}</td>
      <td>${escapeHTML(optionSummary(model.options))}</td>
      <td>${escapeHTML(benchmarkCompactLabel(model))}</td>
      <td>${modelAssetAvailability(model)}</td>
      <td>${routingCell(model)}</td>
      <td><button type="button" data-operation-group="${escapeAttribute(operationGroup)}" data-load-config="${escapeAttribute(model.public_id || model.local_id)}" ${enabled ? "" : "disabled"}>Load</button></td>
    </tr>`;
}

// Only image models can be grouped today, so other rows leave the cell empty
// rather than offering a control that would do nothing.
function routingCell(model: Model): string {
  const imageID = model.image_id;
  if (!imageID || !model.node_id) {
    return "";
  }
  const peers = peerCountForModel(state.routingGroups, {node_id: model.node_id, image_id: imageID});
  return `<button type="button" data-routing-node="${escapeAttribute(model.node_id)}" data-routing-image="${escapeAttribute(imageID)}">${escapeHTML(routingButtonLabel(peers))}</button>`;
}

function modelAssetAvailability(model: Model): string {
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
  return `<span class="asset-badge asset-${escapeAttribute(assetState)}">${escapeHTML(label)}</span>`;
}

function renderSelect(select: HTMLSelectElement, allLabel: string, values: string[], selected: string): void {
  select.innerHTML = `<option value="">${escapeHTML(allLabel)}</option>${values.map(value => `<option value="${escapeAttribute(value)}"${value === selected ? " selected" : ""}>${escapeHTML(value)}</option>`).join("")}`;
}

function scanNotices(nodes: NodeInventory[]): string {
  const failed = nodes.filter(node => node.error);
  if (failed.length === 0) {
    return "";
  }
  return failed.map(node => `<div class="inventory-notice error-text">${escapeHTML(node.node_id || node.node_url || "unknown node")}: ${escapeHTML(node.error || "scan failed")}</div>`).join("");
}
