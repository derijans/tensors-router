import { laneMetadata } from "../../../domain/lanes";
import { cloneOptions, defaultFieldValue, parseOptionInput } from "../../../domain/option-values";
import { emptyHTML, setHTML } from "../../../lib/safe-html";
import { state } from "../../../state/app-state";
import { allOptionDefinitions, nodeByID, optionDefinition } from "../../../state/inventory-selectors";
import type { ConstructorFieldEditor, LaneKind, PaletteComponentPayload } from "../../../types";
import { cookElements } from "../cook-elements";
import { invalidateAcceptedConversions, recordConversion } from "../conversions";
import {
  addableSectionKeys,
  assignedPayload,
  changedDraftValues,
  editorFieldKeys,
  fieldDiff,
  fieldPresetID,
  rawFileKeysForLane,
  requiresOptionAssignment,
  sourceOptionsForPayload,
  validAssignmentKey
} from "./constructor-field-data";
import { loadFieldPresets, presetsForLane, storeFieldPreset } from "./constructor-field-presets";
import { fieldEditorMarkup } from "./constructor-field-view";

type FieldEditorAction = (editor: ConstructorFieldEditor, button: HTMLElement) => void;

const fieldEditorActions: Record<string, FieldEditorAction> = {
  "reset-section": editor => {
    editor.draft = {};
    renderFieldEditor();
  },
  "reset-field": (editor, button) => {
    delete editor.draft[button.dataset.fieldKey || ""];
    renderFieldEditor();
  },
  "add-field": addEditorField,
  "apply-preset": applySelectedPreset,
  "save-preset": savePresetFromEditor
};

export function openFieldEditor(lane: LaneKind, pendingPayload?: PaletteComponentPayload): void {
  loadFieldPresets();
  state.constructor.fieldEditor = {lane, draft: cloneOptions(state.constructor.laneOptions[lane])};
  if (pendingPayload) {
    state.constructor.fieldEditor.pendingPayload = pendingPayload;
  }
  renderFieldEditor();
  if (!cookElements.constructorFieldDialog.open) {
    cookElements.constructorFieldDialog.showModal();
  }
}

export function closeFieldEditor(): void {
  state.constructor.fieldEditor = null;
  cookElements.constructorFieldDialog.close();
  setHTML(cookElements.constructorFieldDialogBody, emptyHTML);
}

export function renderFieldEditor(): void {
  const editor = state.constructor.fieldEditor;
  if (!editor) {
    if (cookElements.constructorFieldDialog.open) {
      closeFieldEditor();
    }
    return;
  }
  const lane = editor.lane;
  const source = sourceOptionsForPayload(editorPayload(editor));
  const definitions = allOptionDefinitions();
  const fieldKeys = editorFieldKeys(lane, source, editor.draft, definitions, optionDefinition);
  const pending = editor.pendingPayload;
  setHTML(cookElements.constructorFieldDialogBody, fieldEditorMarkup({
    metadata: laneMetadata[lane],
    assignment: pending && requiresOptionAssignment(pending, lane) ? {payload: pending, optionKeys: rawFileKeysForLane(lane)} : null,
    presets: presetsForLane(lane, editorBackendMode(editor)),
    addableKeys: addableSectionKeys(lane, fieldKeys, definitions),
    diffs: fieldKeys.map(key => fieldDiff(key, optionDefinition(key)?.name || key, source[key], editor.draft))
  }));
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
  const button = target instanceof HTMLElement ? target.closest<HTMLElement>("[data-field-modal-action]") : null;
  const editor = state.constructor.fieldEditor;
  const action = button?.dataset.fieldModalAction;
  if (!button || !action) {
    return;
  }
  if (action === "cancel") {
    closeFieldEditor();
    return;
  }
  if (action === "apply") {
    if (editor) {
      applyFieldEditor(editor);
    }
    afterApply();
    return;
  }
  if (editor) {
    fieldEditorActions[action]?.(editor, button);
  }
}

function applyFieldEditor(editor: ConstructorFieldEditor): void {
  if (editor.pendingPayload) {
    const optionKey = dialogSelectValue("[data-file-option-key]");
    if (!validAssignmentKey(editor.lane, optionKey)) {
      cookElements.constructorFieldDialogBody.querySelector("[data-file-option-key]")?.setAttribute("aria-invalid", "true");
      return;
    }
    state.constructor.lanes[editor.lane] = assignedPayload(editor.pendingPayload, optionKey);
  }
  state.constructor.laneOptions[editor.lane] = changedDraftValues(editor.draft, sourceOptionsForPayload(state.constructor.lanes[editor.lane]));
  invalidateAcceptedConversions();
  closeFieldEditor();
}

function addEditorField(editor: ConstructorFieldEditor): void {
  const key = dialogSelectValue("[data-field-add-select]");
  if (!key) {
    return;
  }
  editor.draft[key] = defaultFieldValue(optionDefinition(key));
  renderFieldEditor();
}

function applySelectedPreset(editor: ConstructorFieldEditor): void {
  const presetID = dialogSelectValue("[data-field-preset-select]");
  const preset = presetsForLane(editor.lane, editorBackendMode(editor)).find(item => fieldPresetID(item) === presetID);
  if (!presetID || !preset) {
    return;
  }
  Object.assign(editor.draft, cloneOptions(preset.values));
  invalidateAcceptedConversions();
  renderFieldEditor();
}

function savePresetFromEditor(editor: ConstructorFieldEditor): void {
  const input = cookElements.constructorFieldDialogBody.querySelector("[data-field-preset-name]");
  const name = input instanceof HTMLInputElement ? input.value.trim() : "";
  if (!name) {
    return;
  }
  storeFieldPreset({
    name,
    backendMode: editorBackendMode(editor),
    section: laneMetadata[editor.lane].section,
    values: cloneOptions(editor.draft)
  });
  renderFieldEditor();
}

function editorPayload(editor: ConstructorFieldEditor): PaletteComponentPayload | null | undefined {
  return editor.pendingPayload ?? state.constructor.lanes[editor.lane];
}

function editorBackendMode(editor: ConstructorFieldEditor): string {
  const payload = editorPayload(editor);
  return payload?.model?.backend_mode || nodeByID(payload?.component.node_id || "")?.backend_mode || "unknown";
}

function dialogSelectValue(selector: string): string {
  const select = cookElements.constructorFieldDialogBody.querySelector(selector);
  return select instanceof HTMLSelectElement ? select.value : "";
}
