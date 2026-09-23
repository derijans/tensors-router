import { SafeHTML, emptyHTML, html, listOrFallback, setHTML } from "./safe-html";
import { compareOptionKeys, jinjaKwargsPrecedenceKey, jinjaKwargsPrecedenceLabels, laneMetadata } from "./constants";
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
import { invalidateAcceptedConversions, recordConversion } from "./conversions";
import {
  chip,
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
  setHTML(elements.constructorFieldDialogBody, emptyHTML);
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
  setHTML(elements.constructorFieldDialogBody, html`
    <div class="field-dialog-head">
      <div>
        <h3>${metadata.label} Fields</h3>
        <p class="muted">${metadata.section} staged overrides</p>
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
      ${listOrFallback(fieldKeys.map(key => fieldDiffRow(key, source[key], editor.draft)), html`<div class="detail-empty">No fields in this section</div>`)}
    </div>
    <div class="field-dialog-actions">
      <button type="button" data-field-modal-action="reset-section">Reset Section</button>
      <span></span>
      <button type="button" data-field-modal-action="cancel">Cancel</button>
      <button type="button" data-field-modal-action="apply">Apply</button>
    </div>
  `);
}

export function handleFieldEditorInput(target: EventTarget | null): void {
  const editor = state.constructor.fieldEditor;
  if (!editor || (!(target instanceof HTMLInputElement) && !(target instanceof HTMLSelectElement))) {
    return;
  }
  const key = target.dataset.fieldDraft;
  if (!key) {
    return;
  }
  try {
    const parsed = parseOptionInput(optionDefinition(key), target.value);
    editor.draft[key] = parsed.value;
    recordConversion(`lane-${editor.lane}`, key, parsed);
    target.setCustomValidity("");
    renderFieldEditor();
  } catch {
    target.setCustomValidity("Invalid value");
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
  invalidateAcceptedConversions();
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
  invalidateAcceptedConversions();
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

function fieldDiffRow(key: string, sourceValue: JsonValue | undefined, draft: Options): SafeHTML {
  const definition = optionDefinition(key);
  const effectiveSourceValue = key === jinjaKwargsPrecedenceKey && (sourceValue === undefined || sourceValue === null)
    ? "config"
    : sourceValue;
  const hasOverride = Object.hasOwn(draft, key);
  const draftValue = hasOverride ? draft[key] : undefined;
  const changed = hasOverride && comparableJsonValue(draftValue) !== comparableJsonValue(effectiveSourceValue);
  const input = key === jinjaKwargsPrecedenceKey
    ? jinjaKwargsPrecedenceInput(key, hasOverride ? draftValue : effectiveSourceValue)
    : html`<input data-field-draft="${key}" value="${hasOverride ? optionInputValue(draftValue) : ""}" placeholder="inherit">`;
  return html`
    <div class="field-diff-row ${changed ? "changed" : ""}">
      <div class="field-label">
        <span>${definition?.name || key}</span>
        <code>${key}</code>
      </div>
      <div class="field-source">
        <span class="muted">Source</span>
        <strong>${optionValueLabel(effectiveSourceValue) || "inherit"}</strong>
      </div>
      <label class="field-override">
        Override
        ${input}
      </label>
      <div class="field-state">
        ${hasOverride ? chip(changed ? "changed" : "same", changed ? "amber" : "violet") : chip("source", "")}
        <button class="icon-button" type="button" title="Reset field" data-field-modal-action="reset-field" data-field-key="${key}">x</button>
      </div>
    </div>
  `;
}

function jinjaKwargsPrecedenceInput(key: string, value: JsonValue | undefined): SafeHTML {
  const selectedValue = value === "client" ? "client" : "config";
  return html`<select data-field-draft="${key}">${Object.entries(jinjaKwargsPrecedenceLabels).map(([precedence, label]) => html`<option value="${precedence}"${precedence === selectedValue ? " selected" : ""}>${label}</option>`)}</select>`;
}

function assignmentBlock(lane: LaneKind, payload: PaletteComponentPayload): SafeHTML {
  if (!requiresOptionAssignment(payload, lane)) {
    return emptyHTML;
  }
  const keys = rawFileKeysForLane(lane);
  return html`
    <div class="assignment-panel">
      <div>
        <strong>${payload.label}</strong>
        <p class="muted">${payload.subtitle}</p>
      </div>
      <label>
        Assign file to
        <select data-file-option-key>
          ${keys.map(key => html`<option value="${key}">${key}</option>`)}
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
  return Array.from(keys).sort(compareOptionKeys);
}

function addFieldOptions(lane: LaneKind, usedKeys: string[]): SafeHTML {
  const used = new Set(usedKeys);
  const section = laneMetadata[lane].section;
  return html`${allOptionDefinitions()
    .filter(definition => (definition.section || "other") === section && !used.has(definition.key))
    .sort(compareDefinitions)
    .map(definition => html`<option value="${definition.key}">${definition.key}</option>`)}`;
}

function presetOptions(lane: LaneKind): SafeHTML {
  return html`${matchingPresets(lane)
    .map(preset => html`<option value="${fieldPresetID(preset)}">${preset.name}</option>`)}`;
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
  return compareOptionKeys(left.key, right.key);
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
