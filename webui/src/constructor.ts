import { elements } from "./elements";
import { backendModeKey, backendModeLabels, backendModes, compareOptionKeys, isLaneKind, jinjaKwargsKey, jinjaKwargsPrecedenceKey, jinjaKwargsPrecedenceLabels, laneKinds, laneMetadata, type BackendMode } from "./constants";
import { requiresOptionAssignment } from "./constructor-field-data";
import { emptyLaneOptions, emptyLanes, emptyLaneTargets, state } from "./state";
import { openFieldEditor, renderFieldEditor } from "./constructor-field-editor";
import {
  configPaletteEntries,
  filePaletteEntries,
  nodeByID,
  optionDefinition,
  optionPaletteEntries,
  selectedOptionsForInspector,
  usedPaths
} from "./data";
import { localValidation } from "./constructor-data";
import { clearConstructorConversions, clearConversionScope, discardConversion, invalidateAcceptedConversions, recordConversion } from "./conversions";
import {
  chip,
  escapeAttribute,
  escapeHTML,
  kindColor,
  optionInputValue,
  optionValueLabel,
  parseOptionInput,
  renderIssue
} from "./utils";
import type { JsonValue, LaneKind, OptionDefinition, PaletteComponentPayload, PaletteEntry, PalettePayload } from "./types";

export function renderConstructor(): void {
  renderBackendSelector();
  renderPalette();
  renderLanes();
  renderInspector();
  renderFieldEditor();
}

export function addPayload(payload: PalettePayload | undefined, lane?: string): void {
  if (!payload) {
    return;
  }
  if (payload.type === "option") {
    addOption(payload.key);
    return;
  }
  const targetLane = isLaneKind(lane) ? lane : payload.lane;
  if (targetLane !== payload.lane) {
    return;
  }
  if (requiresOptionAssignment(payload, targetLane)) {
    openFieldEditor(targetLane, payload);
    return;
  }
  state.constructor.lanes[targetLane] = payload;
  invalidateAcceptedConversions();
  renderConstructor();
}

export function addOption(key: string): void {
  const definition = optionDefinition(key);
  if (!definition) {
    return;
  }
  if (!Object.hasOwn(state.constructor.options, key)) {
    state.constructor.options[key] = defaultOptionValue(definition);
  }
  if (key === jinjaKwargsKey && !Object.hasOwn(state.constructor.options, jinjaKwargsPrecedenceKey)) {
    state.constructor.options[jinjaKwargsPrecedenceKey] = "config";
  }
  invalidateAcceptedConversions();
  renderConstructor();
}

export function clearConstructor(): void {
  state.constructor.lanes = emptyLanes();
  state.constructor.targetNodes = emptyLaneTargets();
  state.constructor.laneOptions = emptyLaneOptions();
  state.constructor.backendMode = "kobold";
  state.constructor.backendTouched = false;
  state.constructor.options = {};
  state.constructor.fieldEditor = null;
  clearConstructorConversions();
  invalidateAcceptedConversions();
  renderConstructor();
}

export function clearLane(lane: string): void {
  if (!isLaneKind(lane)) {
    return;
  }
  state.constructor.lanes[lane] = null;
  state.constructor.laneOptions[lane] = {};
  clearConversionScope(`lane-${lane}`);
  invalidateAcceptedConversions();
  renderConstructor();
}

export function editLaneFields(lane: string): void {
  if (!isLaneKind(lane) || !state.constructor.lanes[lane]) {
    return;
  }
  openFieldEditor(lane);
}

export function updateOptionInput(target: EventTarget | null): void {
  if (!(target instanceof HTMLInputElement) && !(target instanceof HTMLSelectElement)) {
    return;
  }
  const key = target.dataset.optionInput;
  if (!key) {
    return;
  }
  const parsed = parseOptionInput(optionDefinition(key), target.value);
  state.constructor.options[key] = parsed.value;
  recordConversion("constructor", key, parsed);
  target.setCustomValidity("");
  renderValidation();
}

export function removeOption(key: string): void {
  delete state.constructor.options[key];
  if (key === jinjaKwargsKey) {
    delete state.constructor.options[jinjaKwargsPrecedenceKey];
  }
  discardConversion("constructor", key);
  invalidateAcceptedConversions();
  renderConstructor();
}

export function toggleInspectorList(target: string): void {
  if (target === "used") {
    state.constructor.showUsedAll = !state.constructor.showUsedAll;
  }
  if (target === "options") {
    state.constructor.showOptionsAll = !state.constructor.showOptionsAll;
  }
  renderInspector();
}

export function updateLaneTarget(target: EventTarget | null): void {
  if (!(target instanceof HTMLSelectElement)) {
    return;
  }
  const lane = target.dataset.laneTarget;
  if (!isLaneKind(lane)) {
    return;
  }
  state.constructor.targetNodes[lane] = target.value;
  invalidateAcceptedConversions();
  renderConstructor();
}

export function updateConstructorBackendMode(value: string): void {
  if (!backendModes.includes(value as BackendMode)) {
    return;
  }
  state.constructor.backendMode = value;
  state.constructor.backendTouched = true;
  invalidateAcceptedConversions();
  renderConstructor();
}

function renderBackendSelector(): void {
  const value = constructorBackendModeValue();
  elements.advancedBackendSelect.innerHTML = backendModes.map(mode => {
    const selected = mode === value ? " selected" : "";
    return `<option value="${escapeAttribute(mode)}"${selected}>${escapeHTML(backendModeLabels[mode])}</option>`;
  }).join("");
  elements.advancedBackendSelect.classList.toggle("virtual-backend-select", !state.constructor.backendTouched);
}

function renderPalette(): void {
  const query = elements.constructorFilterInput.value.trim().toLowerCase();
  const entries = paletteEntries().filter(entry => !query || JSON.stringify(entry).toLowerCase().includes(query));
  state.palettePayloads = {};
  elements.paletteList.innerHTML = entries.map(entry => {
    const payloadID = `payload-${Object.keys(state.palettePayloads).length}`;
    state.palettePayloads[payloadID] = entry.payload;
    const addButton = entry.payload.type === "option"
      ? `<button type="button" data-add-option="${escapeAttribute(entry.payload.key)}">Add</button>`
      : `<button type="button" data-select-payload="${escapeAttribute(payloadID)}">Use</button>`;
    return `
      <article class="palette-item" draggable="true" data-drag-payload="${escapeAttribute(payloadID)}">
        <div class="palette-title">
          <strong>${escapeHTML(entry.title)}</strong>
          ${chip(entry.badge, entry.color)}
        </div>
        <div class="muted">${escapeHTML(entry.subtitle)}</div>
        <div class="palette-meta">${entry.meta.map(item => chip(item, "")).join("")}</div>
        ${addButton}
      </article>
    `;
  }).join("") || `<div class="detail-empty">No items</div>`;
}

function renderLanes(): void {
  elements.constructorLanes.innerHTML = laneKinds.map(laneShell).join("");
  for (const lane of laneKinds) {
    const drop = document.querySelector(`[data-drop-lane="${lane}"]`);
    if (!(drop instanceof HTMLElement)) {
      continue;
    }
    const selected = state.constructor.lanes[lane];
    if (!selected) {
      drop.innerHTML = `<div class="lane-empty">${escapeHTML(laneMetadata[lane].dropLabel)}</div>`;
      continue;
    }
    const overrideCount = Object.keys(state.constructor.laneOptions[lane] ?? {}).length;
    drop.innerHTML = `
      <article class="selected-card">
        <strong>${escapeHTML(selected.label)}</strong>
        <div class="muted">${escapeHTML(selected.subtitle)}</div>
        <div class="palette-meta">${selected.meta.map(item => chip(item, "")).join("")}</div>
        ${selected.component.option_key ? `<div class="muted">Assigned to ${escapeHTML(selected.component.option_key)}</div>` : ""}
        <label>
          Target node
          <select data-lane-target="${escapeAttribute(lane)}">${targetNodeOptions(lane, selected)}</select>
        </label>
        <div class="lane-card-actions">
          <button type="button" data-edit-lane-fields="${escapeAttribute(lane)}">Edit fields</button>
          ${overrideCount ? chip(`${overrideCount} overrides`, laneMetadata[lane].accent) : ""}
        </div>
      </article>
    `;
  }
}

function renderInspector(): void {
  renderValidation();
  const used = usedModelRows();
  elements.usedModelsList.innerHTML = limitedRows(used, state.constructor.showUsedAll, "used").join("") || `<div class="detail-empty">No models selected</div>`;
  const options = selectedOptionRows();
  elements.selectedOptionsList.innerHTML = limitedRows(options, state.constructor.showOptionsAll, "options").join("") || `<div class="detail-empty">No options selected</div>`;
}

function renderValidation(): void {
  const validation = localValidation();
  elements.validationList.innerHTML = validation.length
    ? validation.map(renderIssue).join("")
    : `<div class="detail-empty">Clean</div>`;
}

function paletteEntries(): PaletteEntry[] {
  if (state.activePalette === "files") {
    return filePaletteEntries();
  }
  if (state.activePalette === "options") {
    return optionPaletteEntries();
  }
  return configPaletteEntries();
}

function usedModelRows(): string[] {
  const rows: string[] = [];
  for (const lane of laneKinds) {
    const selected = state.constructor.lanes[lane];
    if (!selected) {
      continue;
    }
    rows.push(`
      <div class="used-row">
        ${chip(laneMetadata[lane].shortLabel, kindColor(lane))}
        <span>${escapeHTML(selected.label)}</span>
      </div>
    `);
    for (const value of usedPaths(selected)) {
      rows.push(`<div class="muted">${escapeHTML(value)}</div>`);
    }
  }
  return rows;
}

function selectedOptionRows(): string[] {
  const rows: string[] = [];
  const merged = selectedOptionsForInspector();
  for (const [key, value] of Object.entries(merged).sort(([left], [right]) => compareOptionKeys(left, right))) {
    if (Object.hasOwn(state.constructor.options, key)) {
      rows.push(optionEditorRow(key, state.constructor.options[key]));
    } else if (laneOverrideForKey(key)) {
      const lane = laneOverrideForKey(key);
      rows.push(`
        <div class="option-row">
          ${chip(key, "")}
          ${lane ? chip(`${laneMetadata[lane].shortLabel} override`, laneMetadata[lane].accent) : ""}
          <span class="muted">${escapeHTML(optionValueLabel(value))}</span>
        </div>
      `);
    } else {
      rows.push(`
        <div class="option-row">
          ${chip(key, "")}
          <span class="muted">${escapeHTML(optionValueLabel(value))}</span>
        </div>
      `);
    }
  }
  return rows;
}

function laneShell(lane: LaneKind): string {
  const metadata = laneMetadata[lane];
  return `
    <section class="lane ${escapeAttribute(metadata.accent)}" data-lane="${escapeAttribute(lane)}">
      <div class="lane-head">
        <div>
          <h3>${escapeHTML(metadata.label)}</h3>
          <span>${escapeHTML(metadata.section)}</span>
        </div>
        <button type="button" data-clear-lane="${escapeAttribute(lane)}">Clear</button>
      </div>
      <div class="lane-drop" data-drop-lane="${escapeAttribute(lane)}"></div>
    </section>
  `;
}

function laneOverrideForKey(key: string): LaneKind | null {
  return laneKinds.find(lane => Object.hasOwn(state.constructor.laneOptions[lane] ?? {}, key)) ?? null;
}

function optionEditorRow(key: string, value: JsonValue | undefined): string {
  if (key === jinjaKwargsPrecedenceKey) {
    const selectedValue = value === "client" ? "client" : "config";
    return `
      <div class="option-editor">
        <span>${escapeHTML(key)}</span>
        <select data-option-input="${escapeAttribute(key)}">${Object.entries(jinjaKwargsPrecedenceLabels).map(([precedence, label]) => `<option value="${escapeAttribute(precedence)}"${precedence === selectedValue ? " selected" : ""}>${escapeHTML(label)}</option>`).join("")}</select>
        <button type="button" data-remove-option="${escapeAttribute(key)}">Remove</button>
      </div>
    `;
  }
  return `
    <div class="option-editor">
      <span>${escapeHTML(key)}</span>
      <input data-option-input="${escapeAttribute(key)}" value="${escapeAttribute(optionInputValue(value))}">
      <button type="button" data-remove-option="${escapeAttribute(key)}">Remove</button>
    </div>
  `;
}

function limitedRows(rows: string[], showAll: boolean, target: string): string[] {
  const limit = 9;
  if (rows.length <= limit || showAll) {
    if (rows.length > limit) {
      return [...rows, `<button class="link-button" type="button" data-toggle-list="${target}">Show less</button>`];
    }
    return rows;
  }
  return [
    ...rows.slice(0, limit),
    `<button class="link-button" type="button" data-toggle-list="${target}">Show all ${rows.length}</button>`
  ];
}

function defaultOptionValue(definition: OptionDefinition): JsonValue {
  switch (definition.value_type) {
    case "bool":
      return false;
    case "number":
      return 0;
    case "json":
      return {};
    default:
      return "";
  }
}

function targetNodeOptions(lane: LaneKind, selected: PaletteComponentPayload): string {
  const nodes = state.inventory?.nodes ?? [];
  const current = state.constructor.targetNodes[lane] || selected.component.node_id || nodes[0]?.node_id || "";
  if (!state.constructor.targetNodes[lane]) {
    state.constructor.targetNodes[lane] = current;
  }
  return nodes.map(node => {
    const selectedAttribute = node.node_id === current ? " selected" : "";
    return `<option value="${escapeAttribute(node.node_id)}"${selectedAttribute}>${escapeHTML(node.node_id || "node")}</option>`;
  }).join("");
}

function constructorBackendModeValue(): string {
  if (state.constructor.backendTouched && backendModes.includes(state.constructor.backendMode as BackendMode)) {
    return state.constructor.backendMode;
  }
  for (const lane of laneKinds) {
    const selected = state.constructor.lanes[lane];
    const sourceBackend = selected?.model?.options?.[backendModeKey];
    if (typeof sourceBackend === "string" && backendModes.includes(sourceBackend as BackendMode)) {
      return sourceBackend;
    }
  }
  for (const lane of laneKinds) {
    const selected = state.constructor.lanes[lane];
    if (!selected) {
      continue;
    }
    const nodeID = state.constructor.targetNodes[lane] || selected.component.node_id || "";
    const nodeBackend = nodeByID(nodeID)?.backend_mode || "";
    if (backendModes.includes(nodeBackend as BackendMode)) {
      return nodeBackend;
    }
  }
  const fallback = state.inventory?.nodes?.[0]?.backend_mode || "kobold";
  return backendModes.includes(fallback as BackendMode) ? fallback : "kobold";
}
