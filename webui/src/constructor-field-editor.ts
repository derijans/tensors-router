import { laneMetadata } from "./constants";
import {
  changedDraftValues,
  cloneOptions,
  comparableJsonValue,
  fieldPresetID,
  rawFileKeysForLane,
  requiresOptionAssignment,
  validAssignmentKey
} from "./constructor-field-data";
import { allOptionDefinitions, nodeByID, optionDefinition } from "./data";
import { elements } from "./elements";
import { state } from "./state";
import { defaultFieldValue } from "./simple-cook-data";
import {
  chip,
  escapeAttribute,
  escapeHTML,
  optionInputValue,
  optionValueLabel,
  parseOptionInput
} from "./utils";
import type { FieldPreset, JsonValue, LaneKind, OptionDefinition, Options, PaletteComponentPayload } from "./types";

const presetStorageKey = "tensors-router.constructorFieldPresets";

export function loadFieldPresets(): void {
  if (state.constructor.fieldPresets.length > 0) {
    return;
  }
  try {
    const parsed = JSON.parse(window.localStorage.getItem(presetStorageKey) || "[]") as unknown;
    state.constructor.fieldPresets = Array.isArray(parsed) ? parsed.filter(isFieldPreset) : [];
  } catch {
    state.constructor.fieldPresets = [];
  }
}

export function openFieldEditor(lane: LaneKind, pendingPayload?: PaletteComponentPayload): void {
  loadFieldPresets();
  state.constructor.fieldEditor = {
    lane,
    draft: cloneOptions(state.constructor.laneOptions[lane])
  };
  if (pendingPayload) {
    state.constructor.fieldEditor.pendingPayload = pendingPayload;
  }
  renderFieldEditor();
  showFieldDialog();
}

export function closeFieldEditor(): void {
  state.constructor.fieldEditor = null;
  elements.constructorFieldDialog.close();
  elements.constructorFieldDialogBody.innerHTML = "";
}

export function renderFieldEditor(): void {
  const editor = state.constructor.fieldEditor;
  if (!editor) {
    if (elements.constructorFieldDialog.open) {
      closeFieldEditor();
    }
    return;
  }
  const lane = editor.lane;
  const metadata = laneMetadata[lane];
  const source = sourceOptionsForEditor(editor.pendingPayload ?? state.constructor.lanes[lane]);
  const fieldKeys = editorFieldKeys(lane, source, editor.draft);
  elements.constructorFieldDialogBody.innerHTML = `
    <div class="field-dialog-head">
      <div>
        <h3>${escapeHTML(metadata.label)} Fields</h3>
        <p class="muted">${escapeHTML(metadata.section)} staged overrides</p>
      </div>
      <button class="icon-button" type="button" title="Close" data-field-modal-action="cancel">x</button>
    </div>
    ${editor.pendingPayload ? assignmentBlock(lane, editor.pendingPayload) : ""}
    <div class="preset-row">
      <label>
        Preset
        <select data-field-preset-select>${presetOptions(lane)}</select>
      </label>
      <button type="button" data-field-modal-action="apply-preset">Apply Preset</button>
      <label>
        Save as
        <input data-field-preset-name type="text" placeholder="Preset name">
      </label>
      <button type="button" data-field-modal-action="save-preset">Save Preset</button>
    </div>
    <div class="field-add-row">
      <label>
        Add section field
        <select data-field-add-select>${addFieldOptions(lane, fieldKeys)}</select>
      </label>
      <button type="button" data-field-modal-action="add-field">Add Field</button>
    </div>
    <div class="field-diff-grid">
      ${fieldKeys.map(key => fieldDiffRow(key, source[key], editor.draft)).join("") || `<div class="detail-empty">No fields in this section</div>`}
    </div>
    <div class="field-dialog-actions">
      <button type="button" data-field-modal-action="reset-section">Reset Section</button>
      <span></span>
      <button type="button" data-field-modal-action="cancel">Cancel</button>
      <button type="button" data-field-modal-action="apply">Apply</button>
    </div>
  `;
}

export function handleFieldEditorInput(target: EventTarget | null): void {
  const editor = state.constructor.fieldEditor;
  if (!editor || !(target instanceof HTMLInputElement)) {
    return;
  }
  const key = target.dataset.fieldDraft;
  if (!key) {
    return;
  }
  try {
    editor.draft[key] = parseOptionInput(optionDefinition(key), target.value);
    target.setCustomValidity("");
    renderFieldEditor();
  } catch {
    target.setCustomValidity("Invalid JSON");
    target.reportValidity();
  }
}

export function handleFieldEditorClick(target: EventTarget | null, afterApply: () => void): void {
  const button = target instanceof HTMLElement ? target.closest("[data-field-modal-action]") : null;
  if (!(button instanceof HTMLElement)) {
    return;
  }
  const action = button.dataset.fieldModalAction;
  if (action === "cancel") {
    closeFieldEditor();
    return;
  }
  if (action === "apply") {
    applyFieldEditor();
    afterApply();
    return;
  }
  if (action === "reset-section") {
    resetEditorSection();
    return;
  }
  if (action === "reset-field") {
    resetEditorField(button.dataset.fieldKey || "");
    return;
  }
  if (action === "add-field") {
    addEditorField();
    return;
  }
  if (action === "apply-preset") {
    applySelectedPreset();
    return;
  }
  if (action === "save-preset") {
    savePresetFromEditor();
  }
}

function applyFieldEditor(): void {
  const editor = state.constructor.fieldEditor;
  if (!editor) {
    return;
  }
  if (editor.pendingPayload) {
    const optionKey = selectedAssignmentKey();
    if (!validAssignmentKey(editor.lane, optionKey)) {
      elements.constructorFieldDialogBody.querySelector("[data-file-option-key]")?.setAttribute("aria-invalid", "true");
      return;
    }
    state.constructor.lanes[editor.lane] = assignedPayload(editor.pendingPayload, optionKey);
  }
  const source = sourceOptionsForEditor(state.constructor.lanes[editor.lane]);
  state.constructor.laneOptions[editor.lane] = changedDraftValues(editor.draft, source);
  closeFieldEditor();
}

function resetEditorSection(): void {
  const editor = state.constructor.fieldEditor;
  if (!editor) {
    return;
  }
  editor.draft = {};
  renderFieldEditor();
}

function resetEditorField(key: string): void {
  const editor = state.constructor.fieldEditor;
  if (!editor) {
    return;
  }
  delete editor.draft[key];
  renderFieldEditor();
}

function addEditorField(): void {
  const editor = state.constructor.fieldEditor;
  const select = elements.constructorFieldDialogBody.querySelector("[data-field-add-select]");
  if (!editor || !(select instanceof HTMLSelectElement) || !select.value) {
    return;
  }
  editor.draft[select.value] = defaultFieldValue(optionDefinition(select.value));
  renderFieldEditor();
}

function applySelectedPreset(): void {
  const editor = state.constructor.fieldEditor;
  const select = elements.constructorFieldDialogBody.querySelector("[data-field-preset-select]");
  if (!editor || !(select instanceof HTMLSelectElement) || !select.value) {
    return;
  }
  const preset = matchingPresets(editor.lane).find(item => fieldPresetID(item) === select.value);
  if (!preset) {
    return;
  }
  Object.assign(editor.draft, cloneOptions(preset.values));
  renderFieldEditor();
}

function savePresetFromEditor(): void {
  const editor = state.constructor.fieldEditor;
  const input = elements.constructorFieldDialogBody.querySelector("[data-field-preset-name]");
  if (!editor || !(input instanceof HTMLInputElement) || !input.value.trim()) {
    return;
  }
  const preset: FieldPreset = {
    name: input.value.trim(),
    backendMode: backendModeForEditor(editor),
    section: laneMetadata[editor.lane].section,
    values: cloneOptions(editor.draft)
  };
  state.constructor.fieldPresets = [
    ...state.constructor.fieldPresets.filter(item => fieldPresetID(item) !== fieldPresetID(preset)),
    preset
  ];
  window.localStorage.setItem(presetStorageKey, JSON.stringify(state.constructor.fieldPresets));
  renderFieldEditor();
}

function fieldDiffRow(key: string, sourceValue: JsonValue | undefined, draft: Options): string {
  const definition = optionDefinition(key);
  const hasOverride = Object.hasOwn(draft, key);
  const draftValue = hasOverride ? draft[key] : undefined;
  const changed = hasOverride && comparableJsonValue(draftValue) !== comparableJsonValue(sourceValue);
  return `
    <div class="field-diff-row ${changed ? "changed" : ""}">
      <div class="field-label">
        <span>${escapeHTML(definition?.name || key)}</span>
        <code>${escapeHTML(key)}</code>
      </div>
      <div class="field-source">
        <span class="muted">Source</span>
        <strong>${escapeHTML(optionValueLabel(sourceValue) || "inherit")}</strong>
      </div>
      <label class="field-override">
        Override
        <input data-field-draft="${escapeAttribute(key)}" value="${escapeAttribute(hasOverride ? optionInputValue(draftValue) : "")}" placeholder="inherit">
      </label>
      <div class="field-state">
        ${hasOverride ? chip(changed ? "changed" : "same", changed ? "amber" : "violet") : chip("source", "")}
        <button class="icon-button" type="button" title="Reset field" data-field-modal-action="reset-field" data-field-key="${escapeAttribute(key)}">x</button>
      </div>
    </div>
  `;
}

function assignmentBlock(lane: LaneKind, payload: PaletteComponentPayload): string {
  if (!requiresOptionAssignment(payload, lane)) {
    return "";
  }
  const keys = rawFileKeysForLane(lane);
  return `
    <div class="assignment-panel">
      <div>
        <strong>${escapeHTML(payload.label)}</strong>
        <p class="muted">${escapeHTML(payload.subtitle)}</p>
      </div>
      <label>
        Assign file to
        <select data-file-option-key>
          ${keys.map(key => `<option value="${escapeAttribute(key)}">${escapeHTML(key)}</option>`).join("")}
        </select>
      </label>
    </div>
  `;
}

function editorFieldKeys(lane: LaneKind, source: Options, draft: Options): string[] {
  const section = laneMetadata[lane].section;
  const keys = new Set<string>();
  for (const definition of allOptionDefinitions()) {
    if ((definition.section || "other") === section) {
      keys.add(definition.key);
    }
  }
  for (const key of [...Object.keys(source), ...Object.keys(draft), ...fallbackKeys(lane)]) {
    const definition = optionDefinition(key);
    if (!definition || (definition.section || "other") === section) {
      keys.add(key);
    }
  }
  return Array.from(keys).sort((left, right) => left.localeCompare(right));
}

function addFieldOptions(lane: LaneKind, usedKeys: string[]): string {
  const used = new Set(usedKeys);
  const section = laneMetadata[lane].section;
  return allOptionDefinitions()
    .filter(definition => (definition.section || "other") === section && !used.has(definition.key))
    .sort(compareDefinitions)
    .map(definition => `<option value="${escapeAttribute(definition.key)}">${escapeHTML(definition.key)}</option>`)
    .join("");
}

function presetOptions(lane: LaneKind): string {
  return matchingPresets(lane)
    .map(preset => `<option value="${escapeAttribute(fieldPresetID(preset))}">${escapeHTML(preset.name)}</option>`)
    .join("");
}

function matchingPresets(lane: LaneKind): FieldPreset[] {
  const editor = state.constructor.fieldEditor;
  const section = laneMetadata[lane].section;
  const backendMode = editor ? backendModeForEditor(editor) : "";
  return state.constructor.fieldPresets.filter(preset =>
    preset.section === section &&
    (!preset.backendMode || preset.backendMode === backendMode)
  );
}

function sourceOptionsForEditor(payload: PaletteComponentPayload | null | undefined): Options {
  return cloneOptions(payload?.model?.options ?? {});
}

function backendModeForEditor(editor: { lane: LaneKind; pendingPayload?: PaletteComponentPayload }): string {
  const payload = editor.pendingPayload ?? state.constructor.lanes[editor.lane];
  if (payload?.model?.backend_mode) {
    return payload.model.backend_mode;
  }
  return nodeByID(payload?.component.node_id || "")?.backend_mode || "unknown";
}

function selectedAssignmentKey(): string {
  const select = elements.constructorFieldDialogBody.querySelector("[data-file-option-key]");
  return select instanceof HTMLSelectElement ? select.value : "";
}

function assignedPayload(payload: PaletteComponentPayload, optionKey: string): PaletteComponentPayload {
  return {
    ...payload,
    component: {
      ...payload.component,
      option_key: optionKey
    }
  };
}

function fallbackKeys(lane: LaneKind): string[] {
  return rawFileKeysForLane(lane);
}

function compareDefinitions(left: OptionDefinition, right: OptionDefinition): number {
  return left.key.localeCompare(right.key);
}

function showFieldDialog(): void {
  if (!elements.constructorFieldDialog.open) {
    elements.constructorFieldDialog.showModal();
  }
}

function isFieldPreset(value: unknown): value is FieldPreset {
  if (!value || typeof value !== "object") {
    return false;
  }
  const record = value as Record<string, unknown>;
  return typeof record.name === "string" &&
    typeof record.backendMode === "string" &&
    typeof record.section === "string" &&
    Boolean(record.values) &&
    typeof record.values === "object" &&
    !Array.isArray(record.values);
}
