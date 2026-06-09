import { deleteConfigFile, errorBody, previewConfigFile, applyConfigFile } from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { allOptionDefinitions, optionDefinition, optionValue } from "./data";
import {
  cloneValue,
  comparisonClass,
  configLabel,
  defaultConfigForNode,
  defaultFieldValue,
  fieldChoices,
  fieldRenderContext,
  groupedFieldKeys,
  nodeLabel,
  safeID,
  sectionLabels,
  sectionModelKeys,
  selectedConfig,
  selectedNode,
  sidebarValueRows,
  suggestedConfigID
} from "./simple-cook-data";
import {
  escapeAttribute,
  escapeHTML,
  optionInputValue,
  optionValueLabel,
  parseOptionInput
} from "./utils";
import type { ConfigFileRequest, ConfigFileResponse, JsonValue, Model, RefreshInventory, SelectChoice } from "./types";

type ConfigSubmitter = (request: ConfigFileRequest) => Promise<ConfigFileResponse>;

export function renderSimpleCook(): void {
  syncSimpleCookSelection();
  renderSimpleSelectors();
  renderAddFieldSelect();
  renderConfigEditor();
  renderFieldSidebar();
}

export function selectSimpleNode(nodeID: string): void {
  state.simpleCook.nodeID = nodeID;
  const node = selectedNode();
  const model = (node?.models ?? [])[0] ?? null;
  loadSimpleConfig(model);
  renderSimpleCook();
}

export function selectSimpleConfig(configID: string): void {
  const node = selectedNode();
  const model = (node?.models ?? []).find(item => item.local_id === configID) ?? null;
  loadSimpleConfig(model);
  renderSimpleCook();
}

export function updateSimpleFieldFilter(value: string): void {
  state.simpleCook.fieldFilter = value;
  renderConfigEditor();
}

export function newSimpleConfig(): void {
  const node = selectedNode();
  state.simpleCook.configID = "";
  state.simpleCook.mode = "new";
  state.simpleCook.fields = defaultConfigForNode(node);
  state.simpleCook.cleanFields = {};
  elements.cookIdInput.value = suggestedConfigID(node, "new-config");
  renderSimpleCook();
}

export function copySimpleConfig(): void {
  const config = selectedConfig();
  state.simpleCook.mode = "copy";
  state.simpleCook.configID = "";
  state.simpleCook.fields = cloneValue(state.simpleCook.fields);
  state.simpleCook.cleanFields = {};
  elements.cookIdInput.value = suggestedConfigID(selectedNode(), `${config?.local_id || "config"}-copy`);
  renderSimpleCook();
}

export function addSelectedSimpleField(): void {
  const key = elements.simpleAddFieldSelect.value;
  if (!key || Object.hasOwn(state.simpleCook.fields, key)) {
    return;
  }
  const definition = optionDefinition(key);
  state.simpleCook.fields[key] = defaultFieldValue(definition);
  renderSimpleCook();
}

export function updateSimpleField(target: EventTarget | null): void {
  if (!(target instanceof HTMLInputElement)) {
    return;
  }
  const key = target.dataset.simpleField;
  if (!key) {
    return;
  }
  state.simpleCook.fields[key] = parseOptionInput(optionDefinition(key), target.value);
  renderSimpleCook();
}

export function removeSimpleField(key: string): void {
  delete state.simpleCook.fields[key];
  if (state.simpleCook.sidebar?.key === key) {
    state.simpleCook.sidebar = null;
  }
  renderSimpleCook();
}

export function showSimpleFieldValues(key: string, type: "field" | "model"): void {
  state.simpleCook.sidebar = {key, type};
  renderFieldSidebar();
}

export async function previewSimpleCook(): Promise<void> {
  await submitSimpleConfig(previewConfigFile);
}

export async function applySimpleCook(refreshInventory: RefreshInventory): Promise<void> {
  const result = await submitSimpleConfig(applyConfigFile);
  if (!result) {
    return;
  }
  state.simpleCook.mode = "edit";
  state.simpleCook.configID = result.id || "";
  state.simpleCook.fields = cloneValue(result.options ?? state.simpleCook.fields);
  state.simpleCook.cleanFields = cloneValue(state.simpleCook.fields);
  await refreshInventory();
}

export async function deleteSimpleConfig(refreshInventory: RefreshInventory): Promise<void> {
  const config = selectedConfig();
  if (!config) {
    return;
  }
  if (!window.confirm(`Delete ${config.filename || config.local_id}?`)) {
    return;
  }
  try {
    await deleteConfigFile({
      node_id: state.simpleCook.nodeID,
      node_url: selectedNode()?.node_url || "",
      id: config.local_id,
      filename: config.filename,
      overwrite: false,
      options: {}
    });
    await refreshInventory();
  } catch (error) {
    showSimpleCookError(error);
  }
}

function syncSimpleCookSelection(): void {
  const nodes = state.inventory?.nodes ?? [];
  if (nodes.length === 0) {
    state.simpleCook.nodeID = "";
    state.simpleCook.configID = "";
    state.simpleCook.fields = {};
    state.simpleCook.cleanFields = {};
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
  const config = selectedConfig();
  const currentNode = selectedNode();
  if (!config && (currentNode?.models ?? []).length > 0) {
    loadSimpleConfig((currentNode?.models ?? [])[0] ?? null);
  }
}

function renderSimpleSelectors(): void {
  const nodes = state.inventory?.nodes ?? [];
  fillSelect(elements.simpleNodeSelect, nodes.map(node => optionValue(node.node_id, nodeLabel(node))));
  elements.simpleNodeSelect.value = state.simpleCook.nodeID;
  const configs = selectedNode()?.models ?? [];
  fillSelect(elements.simpleConfigSelect, configs.map(model => optionValue(model.local_id, configLabel(model))));
  elements.simpleConfigSelect.value = state.simpleCook.configID;
  elements.simpleConfigSelect.disabled = configs.length === 0;
  elements.simpleCopyButton.disabled = Object.keys(state.simpleCook.fields || {}).length === 0;
  elements.simpleDeleteButton.disabled = !selectedConfig();
  elements.simpleFieldFilter.value = state.simpleCook.fieldFilter;
}

function renderAddFieldSelect(): void {
  const fields = state.simpleCook.fields || {};
  const options = allOptionDefinitions()
    .filter(definition => !Object.hasOwn(fields, definition.key))
    .sort((left, right) => `${sectionForDefinition(left)}:${left.key}`.localeCompare(`${sectionForDefinition(right)}:${right.key}`));
  elements.simpleAddFieldSelect.innerHTML = options.map(definition => {
    const label = `${sectionLabels[sectionForDefinition(definition)] || "Other"} / ${definition.key}`;
    return `<option value="${escapeAttribute(definition.key)}">${escapeHTML(label)}</option>`;
  }).join("");
}

function renderConfigEditor(): void {
  const fields = state.simpleCook.fields || {};
  const query = state.simpleCook.fieldFilter.trim().toLowerCase();
  const context = fieldRenderContext();
  const groups = groupedFieldKeys(fields, optionDefinition)
    .map(group => {
      const rows = group.keys
        .filter(key => !query || `${key} ${optionValueLabel(fields[key])}`.toLowerCase().includes(query))
        .map(key => fieldRow(key, fields[key], group.section, context))
        .join("");
      if (!rows) {
        return "";
      }
      return `
        <section class="config-section">
          <h3>${escapeHTML(sectionLabels[group.section] || group.section)}</h3>
          <div class="config-fields">${rows}</div>
        </section>
      `;
    })
    .filter(Boolean);
  elements.simpleConfigEditor.innerHTML = groups.join("") || `<div class="detail-empty">No fields</div>`;
}

function renderFieldSidebar(): void {
  const sidebar = state.simpleCook.sidebar;
  if (!sidebar) {
    elements.simpleFieldSidebar.innerHTML = `<div class="detail-empty">Field values</div>`;
    return;
  }
  const rows = sidebarValueRows(sidebar.key, sidebar.type, optionDefinition, fieldRenderContext());
  elements.simpleFieldSidebar.innerHTML = `
    <div class="field-sidebar-head">
      <div>
        <h3>${escapeHTML(sidebar.key)}</h3>
        <p class="muted">${escapeHTML(sidebar.type === "model" ? "same model file" : "same field")}</p>
      </div>
      <button type="button" data-close-field-sidebar>x</button>
    </div>
    <div class="detail-list">
      ${rows.length ? rows.map(sidebarValueRow).join("") : `<div class="detail-empty">No values</div>`}
    </div>
  `;
}

function fieldRow(key: string, value: JsonValue | undefined, section: string, context: ReturnType<typeof fieldRenderContext>): string {
  const definition = optionDefinition(key);
  const datalistID = `field-values-${safeID(key)}`;
  const choices = fieldChoices(key, definition, context);
  const compareClass = comparisonClass(key, section, context);
  const modelButton = sectionModelKeys[section]
    ? `<button class="icon-button" type="button" title="Same model values" data-field-model-values="${escapeAttribute(key)}">M</button>`
    : "";
  return `
    <div class="config-field ${compareClass}">
      <div class="field-label">
        <span>${escapeHTML(definition?.name || key)}</span>
        <code>${escapeHTML(key)}</code>
      </div>
      <div class="field-control">
        <input data-simple-field="${escapeAttribute(key)}" list="${escapeAttribute(datalistID)}" value="${escapeAttribute(optionInputValue(value))}">
        <datalist id="${escapeAttribute(datalistID)}">
          ${choices.map(choice => `<option value="${escapeAttribute(choice)}"></option>`).join("")}
        </datalist>
      </div>
      <div class="field-buttons">
        <button class="icon-button" type="button" title="Other config values" data-field-values="${escapeAttribute(key)}">V</button>
        ${modelButton}
        <button class="icon-button" type="button" title="Remove field" data-remove-simple-field="${escapeAttribute(key)}">x</button>
      </div>
    </div>
  `;
}

function sidebarValueRow(row: { value: string; config: string }): string {
  return `
    <div class="sidebar-value">
      <strong>${escapeHTML(row.value)}</strong>
      <span class="muted">${escapeHTML(row.config)}</span>
    </div>
  `;
}

async function submitSimpleConfig(submitter: ConfigSubmitter): Promise<ConfigFileResponse | null> {
  try {
    const result = await submitter(simpleConfigRequest());
    elements.cookOutput.textContent = JSON.stringify(result, null, 2);
    return result;
  } catch (error) {
    showSimpleCookError(error);
    return null;
  }
}

function simpleConfigRequest(): ConfigFileRequest {
  const config = selectedConfig();
  const requestedID = elements.cookIdInput.value.trim();
  const editingSameConfig = Boolean(config && requestedID === config.local_id);
  return {
    node_id: state.simpleCook.nodeID,
    node_url: selectedNode()?.node_url || "",
    id: requestedID,
    filename: config?.filename || "",
    overwrite: editingSameConfig || elements.overwriteInput.checked,
    options: cloneValue(state.simpleCook.fields)
  };
}

function loadSimpleConfig(model: Model | null): void {
  state.simpleCook.mode = "edit";
  state.simpleCook.configID = model?.local_id || "";
  state.simpleCook.fields = cloneValue(model?.options ?? {});
  state.simpleCook.cleanFields = cloneValue(model?.options ?? {});
  state.simpleCook.sidebar = null;
  elements.cookIdInput.value = model?.local_id || suggestedConfigID(selectedNode(), "new-config");
}

function fillSelect(select: HTMLSelectElement, options: SelectChoice[]): void {
  const selected = select.value;
  select.innerHTML = options.map(option => `<option value="${escapeAttribute(option.value)}">${escapeHTML(option.label)}</option>`).join("");
  if (Array.from(select.options).some(option => option.value === selected)) {
    select.value = selected;
  }
}

function showSimpleCookError(error: unknown): void {
  elements.cookOutput.textContent = JSON.stringify(errorBody(error), null, 2);
}

function sectionForDefinition(definition: { section?: string }): string {
  return definition.section || "other";
}
