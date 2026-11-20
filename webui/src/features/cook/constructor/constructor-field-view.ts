import { jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { type SafeHTML, emptyHTML, html } from "../../../lib/safe-html";
import type { LaneMetadata } from "../../../domain/lanes";
import type { FieldPreset, JsonValue, PaletteComponentPayload } from "../../../types";
import { badge } from "../../../ui/badge";
import { button, closeButton, primaryButton } from "../../../ui/buttons";
import { emptyState, listOrFallback } from "../../../ui/empty-state";
import { optionElement } from "../../../ui/select";
import { mutedText } from "../../../ui/text";
import { jinjaKwargsPrecedenceOptions } from "../jinja-precedence-options-view";
import { type FieldDiff, fieldPresetID } from "./constructor-field-data";

export interface FieldEditorView {
  metadata: LaneMetadata;
  assignment: {payload: PaletteComponentPayload; optionKeys: string[]} | null;
  presets: FieldPreset[];
  addableKeys: string[];
  diffs: FieldDiff[];
}

export function fieldEditorMarkup(view: FieldEditorView): SafeHTML {
  return html`
    <div class="field-dialog-head">
      <div>
        <h3>${view.metadata.label} Fields</h3>
        ${mutedText(html`${view.metadata.section} staged overrides`, "p")}
      </div>
      ${closeButton("Close", {title: "Close", "data-field-modal-action": "cancel"})}
    </div>
    ${view.assignment ? assignmentBlock(view.assignment.payload, view.assignment.optionKeys) : emptyHTML}
    <div class="preset-row">
      <label>
        Preset
        <select data-field-preset-select>${view.presets.map(preset => optionElement(fieldPresetID(preset), preset.name))}</select>
      </label>
      ${button("Apply Preset", {"data-field-modal-action": "apply-preset"})}
      <label>
        Save as
        <input data-field-preset-name type="text" placeholder="Preset name">
      </label>
      ${button("Save Preset", {"data-field-modal-action": "save-preset"})}
    </div>
    <div class="field-add-row">
      <label>
        Add section field
        <select data-field-add-select>${view.addableKeys.map(key => optionElement(key, key))}</select>
      </label>
      ${button("Add Field", {"data-field-modal-action": "add-field"})}
    </div>
    <div class="field-diff-grid">
      ${listOrFallback(view.diffs.map(fieldDiffRow), emptyState("No fields in this section"))}
    </div>
    <div class="field-dialog-actions">
      ${button("Reset Section", {"data-field-modal-action": "reset-section"})}
      <span></span>
      ${button("Cancel", {"data-field-modal-action": "cancel"})}
      ${primaryButton("Apply", {"data-field-modal-action": "apply"})}
    </div>
  `;
}

function fieldDiffRow(diff: FieldDiff): SafeHTML {
  return html`
    <div class="field-diff-row ${diff.changed ? "changed" : ""}">
      <div class="field-label">
        <span>${diff.label}</span>
        <code>${diff.key}</code>
      </div>
      <div class="field-source">
        ${mutedText("Source")}
        <strong>${diff.sourceText || "inherit"}</strong>
      </div>
      <label class="field-override">
        Override
        ${diff.key === jinjaKwargsPrecedenceKey ? jinjaKwargsPrecedenceInput(diff.key, diff.shownValue) : draftTextInput(diff.key, diff.draftText)}
      </label>
      <div class="field-state">
        ${fieldStateBadge(diff)}
        ${closeButton(`Reset ${diff.key}`, {title: "Reset field", "data-field-modal-action": "reset-field", "data-field-key": diff.key})}
      </div>
    </div>
  `;
}

function draftTextInput(key: string, value: string): SafeHTML {
  return html`<input data-field-draft="${key}" value="${value}" placeholder="inherit">`;
}

function fieldStateBadge(diff: FieldDiff): SafeHTML {
  if (!diff.hasOverride) {
    return badge("source", "neutral");
  }
  return diff.changed ? badge("changed", "warning") : badge("same", "neutral");
}

function jinjaKwargsPrecedenceInput(key: string, value: JsonValue | undefined): SafeHTML {
  const selectedValue = value === "client" ? "client" : "config";
  return html`<select data-field-draft="${key}">${jinjaKwargsPrecedenceOptions(selectedValue)}</select>`;
}

function assignmentBlock(payload: PaletteComponentPayload, optionKeys: string[]): SafeHTML {
  return html`
    <div class="assignment-panel">
      <div>
        <strong>${payload.label}</strong>
        ${mutedText(payload.subtitle, "p")}
      </div>
      <label>
        Assign file to
        <select data-file-option-key>
          ${optionKeys.map(key => optionElement(key, key))}
        </select>
      </label>
    </div>
  `;
}
