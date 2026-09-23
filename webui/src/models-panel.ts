import { SafeHTML, emptyHTML, html, setHTML } from "./safe-html";
import { benchmarkCompactLabel } from "./benchmark-data";
import { elements } from "./elements";
import { filterInventoryModels, modelBackends, modelCapabilities } from "./model-inventory-data";
import { linkCountsForModel, routingButtonLabel } from "./routing-links-data";
import { state } from "./state";
import type { Model, NodeInventory, RoutingEndpoint, RoutingLane } from "./types";
import { capabilities, optionSummary } from "./utils";

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
  setHTML(elements.modelsScanNotices, scanNotices(nodes));
  setHTML(elements.modelsTable, filtered.length > 0 ? html`${filtered.map(modelRow)}` : html`<tr><td class="inventory-empty" colspan="11">No models match the current filters.</td></tr>`);
}

function modelRow(model: Model): SafeHTML {
  const enabled = !model.disabled;
  const operationGroup = `model-state-${model.node_id}-${model.local_id}`;
  return html`
    <tr class="${enabled ? "" : "inventory-row-disabled"}">
      <td title="${model.filename}">${model.public_id || model.local_id}</td>
      <td>${model.node_id || ""}</td>
      <td><label class="model-enabled-switch" title="${enabled ? "Disable model" : "Enable model"}"><input type="checkbox" ${enabled ? "checked" : ""} data-operation-group="${operationGroup}" data-model-enabled-node="${model.node_id}" data-model-enabled-id="${model.local_id}"><span aria-hidden="true"></span><span class="sr-only">${enabled ? "Enabled" : "Disabled"}</span></label></td>
      <td>${model.backend_mode || ""}</td>
      <td>${capabilities(model)}</td>
      <td>${optionSummary(model.options)}</td>
      <td>${benchmarkCompactLabel(model)}</td>
      <td>${modelAssetAvailability(model)}</td>
      <td>${routingCell(model)}</td>
      <td>${separateCell(model, operationGroup)}</td>
      <td><button type="button" data-operation-group="${operationGroup}" data-load-config="${model.public_id || model.local_id}" ${enabled ? "" : "disabled"}>Load</button></td>
    </tr>`;
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
  return html`${buttons.map(button => html`${button} `)}`;
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
  return html`<span class="asset-badge asset-${assetState}">${label}</span>`;
}

function renderSelect(select: HTMLSelectElement, allLabel: string, values: string[], selected: string): void {
  setHTML(select, html`<option value="">${allLabel}</option>${values.map(value => html`<option value="${value}"${value === selected ? " selected" : ""}>${value}</option>`)}`);
}

function scanNotices(nodes: NodeInventory[]): SafeHTML {
  const failed = nodes.filter(node => node.error);
  if (failed.length === 0) {
    return emptyHTML;
  }
  return html`${failed.map(node => html`<div class="inventory-notice error-text">${node.node_id || node.node_url || "unknown node"}: ${node.error || "scan failed"}</div>`)}`;
}
