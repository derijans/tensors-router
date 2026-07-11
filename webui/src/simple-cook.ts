import { deleteConfigFile, errorBody, previewConfigFile, applyConfigFile } from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { clearConversionScope, confirmPendingConversions, discardConversion, invalidateAcceptedConversions, recordConversion } from "./conversions";
import { cookResultHTML } from "./cook-result";
import { confirmDestructive } from "./dialogs";
import { markSimpleCookClean } from "./dirty-state";
import {
  backendModeKey,
  backendModeLabels,
  backendModes,
  unloadPolicies,
  unloadPolicyKey,
  unloadPolicyLabels,
  type BackendMode,
  type UnloadPolicy
} from "./constants";
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
import type { ConfigFileRequest, ConfigFileResponse, JsonValue, Model, Options, RefreshInventory, SelectChoice } from "./types";

type ConfigSubmitter = (request: ConfigFileRequest) => Promise<ConfigFileResponse>;

const primaryRuntimeKeys = [backendModeKey, unloadPolicyKey];

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

export function updateSimpleSectionOpen(target: EventTarget | null): void {
  if (!(target instanceof HTMLDetailsElement)) {
    return;
  }
  const section = target.dataset.simpleSection;
  if (!section) {
    return;
  }
  const openSections = new Set(state.simpleCook.openSections);
  if (target.open) {
    openSections.add(section);
  } else {
    openSections.delete(section);
  }
  state.simpleCook.openSections = Array.from(openSections);
}

export function newSimpleConfig(): void {
  const node = selectedNode();
  state.simpleCook.configID = "";
  state.simpleCook.mode = "new";
  state.simpleCook.fields = defaultConfigForNode(node);
  state.simpleCook.cleanFields = {};
  state.simpleCook.cleanID = "";
  clearConversionScope("quick");
  state.simpleCook.openSections = [];
  elements.cookIdInput.value = suggestedConfigID(node, "new-config");
  renderSimpleCook();
}

export function copySimpleConfig(): void {
  const config = selectedConfig();
  state.simpleCook.mode = "copy";
  state.simpleCook.configID = "";
  state.simpleCook.fields = cloneValue(state.simpleCook.fields);
  state.simpleCook.cleanFields = {};
  state.simpleCook.cleanID = "";
  clearConversionScope("quick");
  state.simpleCook.openSections = [];
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
  invalidateAcceptedConversions();
  renderSimpleCook();
}

export function updateSimpleField(target: EventTarget | null): void {
  if (!(target instanceof HTMLInputElement) && !(target instanceof HTMLSelectElement)) {
    return;
  }
  if (target instanceof HTMLSelectElement && target.dataset.simpleBackendMode !== undefined) {
    state.simpleCook.fields[backendModeKey] = target.value;
    invalidateAcceptedConversions();
    renderSimpleCook();
    return;
  }
  const key = target.dataset.simpleField;
  if (!key) {
    return;
  }
  const parsed = parseOptionInput(optionDefinition(key), target.value);
  state.simpleCook.fields[key] = parsed.value;
  recordConversion("quick", key, parsed);
  renderSimpleCook();
}

export function removeSimpleField(key: string): void {
  delete state.simpleCook.fields[key];
  discardConversion("quick", key);
  invalidateAcceptedConversions();
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
  if (!await confirmPendingConversions("quick")) {
    return;
  }
  await submitSimpleConfig(previewConfigFile);
}

export async function applySimpleCook(refreshInventory: RefreshInventory): Promise<void> {
  if (!await confirmPendingConversions("quick")) {
    return;
  }
  const request = simpleConfigRequest();
  if (request.overwrite && !await confirmDestructive("Overwrite config?", `Applying ${request.id || "this config"} replaces the existing configuration.`, "Overwrite")) {
    return;
  }
  const result = await submitSimpleConfig(applyConfigFile);
  if (!result) {
    return;
  }
  state.simpleCook.mode = "edit";
  state.simpleCook.configID = result.id || "";
  state.simpleCook.fields = cloneValue(result.options ?? state.simpleCook.fields);
  elements.cookIdInput.value = result.id || elements.cookIdInput.value;
  markSimpleCookClean();
  clearConversionScope("quick");
  await refreshInventory();
}

export async function deleteSimpleConfig(refreshInventory: RefreshInventory): Promise<void> {
  const config = selectedConfig();
  if (!config) {
    return;
  }
  if (!await confirmDestructive("Delete config?", `Delete ${config.filename || config.local_id}? This cannot be undone.`, "Delete")) {
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
    throw error;
  }
}

function syncSimpleCookSelection(): void {
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
    .filter(definition => !primaryRuntimeKeys.includes(definition.key) && !Object.hasOwn(fields, definition.key))
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
  const openSections = new Set(state.simpleCook.openSections);
  const groups = simpleCookFieldGroups(fields)
    .map(group => {
      const keys = group.keys
        .filter(key => !query || `${key} ${optionValueLabel(fieldValue(key))}`.toLowerCase().includes(query));
      const rows = keys
        .map(key => fieldRow(key, fieldValue(key), group.section, context, key === backendModeKey && !Object.hasOwn(fields, backendModeKey)))
        .join("");
      if (!rows) {
        return null;
      }
      const sectionLabel = sectionLabels[group.section] || group.section;
      return {
        section: group.section,
        html: `
        <details class="config-section" data-simple-section="${escapeAttribute(group.section)}"${openSections.has(group.section) ? " open" : ""}>
          <summary>
            <span>${escapeHTML(sectionLabel)}</span>
            <span class="section-count">${escapeHTML(fieldCountLabel(keys.length))}</span>
          </summary>
          <div class="config-fields">${rows}</div>
        </details>
      `
      };
    })
    .filter((group): group is { section: string; html: string } => group !== null);
  elements.simpleConfigEditor.innerHTML = groups.length
    ? groups.map(group => group.html).join("")
    : `<div class="detail-empty">No fields</div>`;
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

function fieldRow(key: string, value: JsonValue | undefined, section: string, context: ReturnType<typeof fieldRenderContext>, virtual = false): string {
  const definition = optionDefinition(key);
  const datalistID = `field-values-${safeID(key)}`;
  const choices = fieldChoices(key, definition, context);
  const compareClass = comparisonClass(key, section, context);
  const input = simpleFieldInput(key, value, datalistID, choices, virtual);
  const modelButton = sectionModelKeys[section]
    ? `<button class="icon-button" type="button" title="Same model values" data-field-model-values="${escapeAttribute(key)}">M</button>`
    : "";
  return `
    <div class="config-field ${compareClass}${virtual ? " backend-virtual" : ""}">
      <div class="field-label">
        <span>${escapeHTML(definition?.name || key)}</span>
        <code>${escapeHTML(key)}</code>
      </div>
      <div class="field-control">
        ${input}
      </div>
      <div class="field-buttons">
        <button class="icon-button" type="button" title="Other config values" data-field-values="${escapeAttribute(key)}">V</button>
        ${modelButton}
        ${virtual ? "" : `<button class="icon-button" type="button" title="Remove field" data-remove-simple-field="${escapeAttribute(key)}">x</button>`}
      </div>
    </div>
  `;
}

function simpleFieldInput(key: string, value: JsonValue | undefined, datalistID: string, choices: string[], virtual: boolean): string {
  if (key === backendModeKey) {
    return backendModeSelect(simpleBackendModeValue(), virtual);
  }
  if (key === unloadPolicyKey) {
    return unloadPolicySelect(optionInputValue(value), virtual);
  }
  return `
    <input data-simple-field="${escapeAttribute(key)}" list="${escapeAttribute(datalistID)}" value="${escapeAttribute(optionInputValue(value))}">
    <datalist id="${escapeAttribute(datalistID)}">
      ${choices.map(choice => `<option value="${escapeAttribute(choice)}"></option>`).join("")}
    </datalist>
  `;
}

function backendModeSelect(value: string, virtual: boolean): string {
  const selectedValue = backendModes.includes(value as BackendMode) ? value : "kobold";
  return `
    <select data-simple-backend-mode class="${virtual ? "virtual-backend-select virtual-runtime-select" : ""}">
      ${backendModes.map(mode => `<option value="${escapeAttribute(mode)}"${mode === selectedValue ? " selected" : ""}>${escapeHTML(backendModeLabels[mode])}</option>`).join("")}
    </select>
  `;
}

function unloadPolicySelect(value: string, virtual: boolean): string {
  const selectedValue = unloadPolicies.includes(value as UnloadPolicy) ? value : "none";
  const customOption = value && value !== selectedValue
    ? `<option value="${escapeAttribute(value)}" selected>${escapeHTML(value)}</option>`
    : "";
  return `
    <select data-simple-field="${escapeAttribute(unloadPolicyKey)}" class="${virtual ? "virtual-runtime-select" : ""}">
      ${customOption}
      ${unloadPolicies.map(policy => `<option value="${escapeAttribute(policy)}"${policy === selectedValue && !customOption ? " selected" : ""}>${escapeHTML(unloadPolicyLabels[policy])}</option>`).join("")}
    </select>
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
    elements.cookOutput.innerHTML = cookResultHTML(result);
    return result;
  } catch (error) {
    showSimpleCookError(error);
    throw error;
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

function simpleCookFieldGroups(fields: Options) {
  const groups = groupedFieldKeys(fields, optionDefinition)
    .map(group => ({
      ...group,
      keys: group.keys.filter(key => !primaryRuntimeKeys.includes(key))
    }))
    .filter(group => group.keys.length > 0);
  const runtimeGroup = groups.find(group => group.section === "runtime");
  if (runtimeGroup) {
    runtimeGroup.keys = [...primaryRuntimeKeys, ...runtimeGroup.keys];
    return groups;
  }
  return [...groups, {section: "runtime", keys: [...primaryRuntimeKeys]}];
}

function fieldValue(key: string): JsonValue | undefined {
  if (key === backendModeKey && !Object.hasOwn(state.simpleCook.fields, backendModeKey)) {
    return simpleBackendModeValue();
  }
  if (key === unloadPolicyKey && !Object.hasOwn(state.simpleCook.fields, unloadPolicyKey)) {
    return "none";
  }
  return state.simpleCook.fields[key];
}

function simpleBackendModeValue(): string {
  const value = state.simpleCook.fields[backendModeKey];
  if (typeof value === "string" && backendModes.includes(value as BackendMode)) {
    return value;
  }
  const fallback = selectedConfig()?.backend_mode || selectedNode()?.backend_mode || "kobold";
  return backendModes.includes(fallback as BackendMode) ? fallback : "kobold";
}

function loadSimpleConfig(model: Model | null): void {
  state.simpleCook.mode = "edit";
  state.simpleCook.configID = model?.local_id || "";
  state.simpleCook.fields = cloneValue(model?.options ?? {});
  state.simpleCook.cleanFields = cloneValue(model?.options ?? {});
  state.simpleCook.sidebar = null;
  state.simpleCook.openSections = [];
  elements.cookIdInput.value = model?.local_id || suggestedConfigID(selectedNode(), "new-config");
  state.simpleCook.cleanID = elements.cookIdInput.value.trim();
  clearConversionScope("quick");
}

function fillSelect(select: HTMLSelectElement, options: SelectChoice[]): void {
  const selected = select.value;
  select.innerHTML = options.map(option => `<option value="${escapeAttribute(option.value)}">${escapeHTML(option.label)}</option>`).join("");
  if (Array.from(select.options).some(option => option.value === selected)) {
    select.value = selected;
  }
}

function showSimpleCookError(error: unknown): void {
  elements.cookOutput.innerHTML = cookResultHTML(errorBody(error));
}

function sectionForDefinition(definition: { section?: string }): string {
  return definition.section || "other";
}

function fieldCountLabel(count: number): string {
  return count === 1 ? "1 field" : `${count} fields`;
}
